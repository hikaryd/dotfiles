package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

func piApply(root string) error {
	agent := absolute(envDefault([]string{"PI_CODING_AGENT_DIR"}, "~/.pi/agent"))
	var changes []change
	for _, name := range []string{"settings.json", "models.json", "mcp.json"} {
		p := filepath.Join(agent, name)
		original, e := readOptional(p)
		if e != nil {
			return e
		}
		current := map[string]any{}
		if original != nil {
			current, e = object(original)
			if e != nil {
				return fmt.Errorf("invalid JSON object in %s", name)
			}
		}
		template, e := readObject(filepath.Join(root, "config/pi", name), false)
		if e != nil {
			return e
		}
		target, e := resolve(p)
		if e != nil {
			return e
		}
		merged := merge(current, template)
		if name == "mcp.json" {
			servers := map[string]any{}
			for _, config := range []map[string]any{current, template} {
				value, exists := config["mcpServers"]
				if !exists {
					continue
				}
				entries, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid mcpServers object in %s", name)
				}
				for server, entry := range entries {
					// Replace managed entries as a unit: a recursive merge could
					// retain a stale HTTP transport or credentials on a stdio server.
					servers[server] = entry
				}
			}
			merged["mcpServers"] = servers
		}
		updated := encode(merged)
		if original != nil && reflect.DeepEqual(current, merged) {
			updated = original
		}
		c := change{path: target, data: updated, backup: target + ".bak-" + stamp(), expect: true, expected: original}
		changes = append(changes, c)
	}
	for _, r := range [][2]string{{"themes/ayu.json", "themes/ayu.json"}, {"extensions/dots-ayu-bar", "extensions/ayu-bar"}, {"extensions/dots-task-titles", "extensions/task-titles"}} {
		source := absolute(filepath.Join(root, "config/pi", r[1]))
		if _, e := os.Stat(source); e != nil {
			return fmt.Errorf("missing Pi resource: %s", r[1])
		}
		p := filepath.Join(agent, r[0])
		changes = append(changes, change{path: p, link: source, backup: filepath.Join(agent, "backups", filepath.Base(p)+".bak-"+stamp())})
	}
	if e := commit(changes); e != nil {
		return e
	}
	fmt.Printf("Pi: personal settings, MCP servers, Ayu theme, quota bar, Herdr titles and model overrides installed in %s\n", agent)
	return nil
}
