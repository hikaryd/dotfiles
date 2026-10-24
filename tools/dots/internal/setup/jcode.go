package setup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// jcodeApply publishes owned MCP entries, the single-Codex policy and shared theme.
// Personal model/authentication, hooks and unrelated MCP servers are preserved.
func jcodeApply(root string) error {
	configDir := absolute(envDefault([]string{"JCODE_CONFIG_DIR"}, filepath.Join(home(), ".jcode")))
	dataDir := absolute(envDefault([]string{"JCODE_INTEGRATIONS_DIR"}, filepath.Join(envDefault([]string{"XDG_DATA_HOME"}, filepath.Join(home(), ".local/share")), "jcode-integrations")))
	node := envDefault([]string{"JCODE_NODE_BIN"}, "node")
	if node == "node" {
		resolved, err := exec.LookPath(node)
		if err != nil {
			return errors.New("Node.js is required for jcode integrations")
		}
		node = absolute(resolved)
	}
	configPath, mcpPath := filepath.Join(configDir, "config.toml"), filepath.Join(configDir, "mcp.json")
	priorConfig, err := regularOptional(configPath)
	if err != nil {
		return err
	}
	priorMCP, err := regularOptional(mcpPath)
	if err != nil {
		return err
	}
	current := map[string]any{}
	if len(priorConfig) > 0 {
		if err = toml.Unmarshal(priorConfig, &current); err != nil {
			return errors.New("invalid jcode config.toml")
		}
	}
	for _, name := range []string{"provider", "features", "agents", "hooks", "display"} {
		if value, exists := current[name]; exists {
			if _, ok := value.(map[string]any); !ok {
				return errors.New("jcode settings must use tables")
			}
		} else {
			current[name] = map[string]any{}
		}
	}
	currentMCP := map[string]any{}
	if priorMCP != nil {
		currentMCP, err = object(priorMCP)
		if err != nil {
			return errors.New("invalid jcode mcp.json")
		}
	}
	serverKey := "mcpServers"
	if _, historical := currentMCP["servers"]; historical {
		if _, duplicate := currentMCP["mcpServers"]; duplicate {
			return errors.New("jcode MCP config must not contain both servers and mcpServers")
		}
		serverKey = "servers"
	}
	servers := map[string]any{}
	if value, exists := currentMCP[serverKey]; exists {
		var ok bool
		servers, ok = value.(map[string]any)
		if !ok {
			return errors.New("jcode MCP server map must be an object")
		}
	}
	desired, err := readObject(filepath.Join(root, "config/jcode/mcp.json"), false)
	if err != nil {
		return errors.New("invalid jcode MCP template")
	}
	managed, ok := desired["mcpServers"].(map[string]any)
	names := []string{"serena", "context7", "deepwiki", "code-graph", "neo4j-cypher", "neo4j-memory", "piweb"}
	if !ok || len(managed) != len(names) {
		return errors.New("jcode MCP template must define the seven managed servers")
	}
	for _, name := range names {
		if _, ok := managed[name].(map[string]any); !ok {
			return errors.New("invalid managed jcode MCP server")
		}
	}
	themeData, err := os.ReadFile(filepath.Join(root, "config/jcode/theme.toml"))
	if err != nil {
		return errors.New("cannot read shared jcode theme")
	}
	var theme struct {
		Display struct {
			Theme  string
			Colors map[string]string
		}
	}
	if err := toml.Unmarshal(themeData, &theme); err != nil || theme.Display.Theme != "dark" || len(theme.Display.Colors) == 0 {
		return errors.New("invalid shared jcode theme")
	}
	for _, color := range theme.Display.Colors {
		if len(color) != 7 || color[0] != '#' {
			return errors.New("invalid shared jcode theme color")
		}
		if _, err := hex.DecodeString(color[1:]); err != nil {
			return errors.New("invalid shared jcode theme color")
		}
	}
	password, err := jcodeNeo4jPassword(dataDir)
	if err != nil {
		return err
	}
	path := filepath.Dir(node) + string(os.PathListSeparator) + envDefault([]string{"PATH"}, "/usr/bin:/bin")
	replacer := strings.NewReplacer("@NODE@", node, "@DATA@", dataDir, "@PATH@", path, "@NEO4J_PASSWORD@", password)
	managed = replaceJcodeTokens(managed, replacer).(map[string]any)
	delete(servers, "pilens")
	for _, name := range names {
		servers[name] = managed[name]
	}
	currentMCP[serverKey] = servers
	provider := current["provider"].(map[string]any)
	provider["default_provider"], provider["cross_provider_failover"] = "openai", "manual"
	features := current["features"].(map[string]any)
	features["swarm"], features["auto_poke"], features["memory"] = false, false, false
	agents := current["agents"].(map[string]any)
	agents["memory_sidecar_enabled"], agents["swarm_max_concurrent_agents"] = false, int64(1)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }
	if hooks := current["hooks"].(map[string]any); hooks["post_tool_feedback"] == quote(node)+" "+quote(filepath.Join(dataDir, "lens/hooks.mjs")) {
		delete(hooks, "post_tool_feedback")
		delete(hooks, "post_tool_feedback_timeout_ms")
	}
	display := current["display"].(map[string]any)
	display["theme"], display["colors"] = theme.Display.Theme, theme.Display.Colors
	config, err := toml.Marshal(current)
	if err != nil {
		return errors.New("cannot encode jcode configuration")
	}
	if err = commit([]change{
		{path: mcpPath, data: encode(currentMCP), mode: 0600, backup: privateBackup("jcode", mcpPath), backupPrivate: true, expect: true, expected: priorMCP},
		{path: configPath, data: config, mode: 0600, backup: privateBackup("jcode", configPath), backupPrivate: true, expect: true, expected: priorConfig},
	}); err != nil {
		return err
	}
	fmt.Println("jcode: original MCP tools connected; Codex only, background agents disabled")
	return nil
}

// jcodeNeo4jPassword never replaces an existing credential or follows symlinks.
func jcodeNeo4jPassword(dataDir string) (string, error) {
	dir := filepath.Join(dataDir, "neo4j")
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode().Perm() != 0700 {
			return "", errors.New("Neo4j credential directory must be private (0700)")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	} else if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "private.env")
	if _, err := os.Lstat(p); os.IsNotExist(err) {
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return "", errors.New("cannot generate Neo4j credential")
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil && !os.IsExist(err) {
			return "", errors.New("cannot create private Neo4j credential")
		}
		if err == nil {
			_, writeErr := fmt.Fprintf(f, "NEO4J_PASSWORD=%s\n", hex.EncodeToString(random))
			closeErr := f.Close()
			if writeErr != nil || closeErr != nil {
				return "", errors.New("cannot save private Neo4j credential")
			}
		}
	}
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return "", errors.New("Neo4j credential must be a regular private file (0600)")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", errors.New("cannot read private Neo4j credential")
	}
	const prefix = "NEO4J_PASSWORD="
	value := strings.TrimSuffix(strings.TrimPrefix(string(data), prefix), "\n")
	decoded, err := hex.DecodeString(value)
	if !strings.HasPrefix(string(data), prefix) || err != nil || len(decoded) != 32 || len(value) != 64 {
		return "", errors.New("invalid private Neo4j credential format")
	}
	return value, nil
}

func replaceJcodeTokens(value any, replacer *strings.Replacer) any {
	switch v := value.(type) {
	case string:
		return replacer.Replace(v)
	case map[string]any:
		for k, item := range v {
			v[k] = replaceJcodeTokens(item, replacer)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = replaceJcodeTokens(item, replacer)
		}
		return v
	default:
		return value
	}
}
