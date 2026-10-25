package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

func piApply(root string) error {
	agent := absolute(envDefault([]string{"PI_CODING_AGENT_DIR"}, "~/.pi/agent"))
	lensHome := absolute(envDefault([]string{"PI_LENS_HOME"}, "~/.pi-lens"))
	lensConfig := absolute(envDefault([]string{"PI_LENS_CONFIG_PATH"}, filepath.Join(lensHome, "config.json")))
	var changes []change
	for _, config := range [][2]string{
		{"settings.json", filepath.Join(agent, "settings.json")},
		{"models.json", filepath.Join(agent, "models.json")},
		{"mcp.json", filepath.Join(agent, "mcp.json")},
		{"web-search.json", filepath.Join(agent, "web-search.json")},
		{"lens.json", lensConfig},
	} {
		name, p := config[0], config[1]
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
		// Supply the managed local proxy by default; keep explicit personal proxies.
		if name == "settings.json" && current["httpProxy"] != nil && current["httpProxy"] != "http://127.0.0.1:10810" {
			merged["httpProxy"] = current["httpProxy"]
		}
		if name == "web-search.json" && current["proxy"] != nil && current["proxy"] != "http://127.0.0.1:10810" {
			merged["proxy"] = current["proxy"]
		}
		if name == "settings.json" {
			// Add managed references without dropping personal packages/extensions/skills
			// or replacing package objects carrying local resource filters.
			for _, key := range []string{"packages", "extensions", "skills"} {
				managed, ok := template[key].([]any)
				if !ok {
					continue
				}
				local, _ := current[key].([]any)
				entries := append([]any{}, local...)
				for _, wanted := range managed {
					found := false
					for _, entry := range entries {
						if pkg, ok := entry.(map[string]any); ok && key == "packages" {
							entry = pkg["source"]
						}
						if reflect.DeepEqual(entry, wanted) {
							found = true
							break
						}
					}
					if !found {
						entries = append(entries, wanted)
					}
				}
				merged[key] = entries
			}
		}
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
	for _, r := range [][2]string{
		{"themes/ayu.json", "themes/ayu.json"},
		{"extensions/dots-ayu-bar", "extensions/ayu-bar"},
		{"extensions/dots-task-titles", "extensions/task-titles"},
		{"extensions/powerline-footer/theme.json", "extensions/powerline-footer/theme.json"},
	} {
		source := absolute(filepath.Join(root, "config/pi", r[1]))
		if _, e := os.Stat(source); e != nil {
			return fmt.Errorf("missing Pi resource: %s", r[1])
		}
		p := filepath.Join(agent, r[0])
		changes = append(changes, change{path: p, link: source, backup: filepath.Join(agent, "backups", filepath.Base(p)+".bak-"+stamp())})
	}
	if repair, e := piSupervisorRepair(agent); e != nil {
		return e
	} else if repair != nil {
		changes = append(changes, *repair)
	}
	if e := commit(changes); e != nil {
		return e
	}
	fmt.Printf("Pi: personal settings, MCP servers, Lens, free web search, Powerline, Ayu theme, Herdr titles and model overrides installed in %s\n", agent)
	return nil
}
