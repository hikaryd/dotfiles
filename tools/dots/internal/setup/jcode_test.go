package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func jcodeFixture(t *testing.T) (string, string) {
	t.Helper()
	h := fixtureHome(t)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("JCODE_CONFIG_DIR", "")
	t.Setenv("JCODE_INTEGRATIONS_DIR", "")
	t.Setenv("JCODE_NODE_BIN", "/fixture/node")
	root := filepath.Join(h, "checkout")
	put(t, filepath.Join(root, "config/jcode/mcp.json"), `{"mcpServers":{"serena":{"command":"@DATA@/python/serena-agent/bin/serena"},"context7":{"command":"@NODE@"},"deepwiki":{"command":"@NODE@"},"code-graph":{"command":"@DATA@/python/codegraphcontext/bin/cgc","env":{"NEO4J_PASSWORD":"@NEO4J_PASSWORD@"}},"neo4j-cypher":{"command":"@DATA@/python/mcp-neo4j-cypher/bin/mcp-neo4j-cypher","env":{"NEO4J_PASSWORD":"@NEO4J_PASSWORD@"}},"neo4j-memory":{"command":"@DATA@/python/mcp-neo4j-memory/bin/mcp-neo4j-memory","env":{"NEO4J_PASSWORD":"@NEO4J_PASSWORD@"}},"agent-voice":{"command":"@HOME@/Library/Application Support/VoiceStudio/runtime/project/.venv/bin/python","args":["-u","@DATA@/voice/server.py"],"env":{"HF_HUB_OFFLINE":"1","TRANSFORMERS_OFFLINE":"1","AGENT_VOICE_CACHE_DIR":"@DATA@/voice/audio"}},"piweb":{"command":"@NODE@","args":["@DATA@/runtime/node_modules/pi-web-access/dist/mcp-cli.js"],"env":{"PI_CODING_AGENT_DIR":"@DATA@/web/config"}}}}`)
	put(t, filepath.Join(root, "config/jcode/agent-voice.py"), "print('fixture voice')\n")
	put(t, filepath.Join(root, "config/jcode/theme.toml"), "[display]\ntheme = 'dark'\n[display.colors]\naccent = '#73d0ff'\n")
	return h, root
}

func TestJcodeApplyPreservesSettingsAndIdempotence(t *testing.T) {
	h, root := jcodeFixture(t)
	configPath := filepath.Join(h, ".jcode/config.toml")
	mcpPath := filepath.Join(h, ".jcode/mcp.json")
	put(t, configPath, "[provider]\ndefault_model = 'personal-model'\n[hooks]\nsession_end = 'personal-hook'\n[launch_hotkeys]\nenabled = true\n")
	put(t, mcpPath, `{"version":7,"mcpServers":{"kaiten":{"command":"personal","env":{"TOKEN":"fixture-secret"}},"pilens":{"command":"old"}}}`)
	authPath := filepath.Join(h, ".jcode/auth.json")
	put(t, authPath, "private credentials")
	if code, out, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 || strings.Contains(out+diagnostic, "fixture-secret") {
		t.Fatal(code, out, diagnostic)
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(get(t, configPath)), &config); err != nil {
		t.Fatal(err)
	}
	if config["provider"].(map[string]any)["default_model"] != "personal-model" || config["hooks"].(map[string]any)["session_end"] != "personal-hook" || config["launch_hotkeys"].(map[string]any)["enabled"] != true {
		t.Fatal(config)
	}
	if _, exists := config["hooks"].(map[string]any)["post_tool_feedback"]; exists {
		t.Fatal("minimal setup must not add automatic feedback")
	}
	mcp, err := readObject(mcpPath, false)
	if err != nil {
		t.Fatal(err)
	}
	servers := mcp["mcpServers"].(map[string]any)
	if len(servers) != 9 || servers["kaiten"].(map[string]any)["command"] != "personal" || servers["context7"].(map[string]any)["command"] != "/fixture/node" || servers["pilens"] != nil {
		t.Fatal("MCP merge failed")
	}
	if get(t, authPath) != "private credentials" {
		t.Fatal("authentication changed")
	}
	for _, p := range []string{configPath, mcpPath} {
		info, _ := os.Stat(p)
		if info.Mode().Perm() != 0600 {
			t.Fatal("configuration not private", p)
		}
	}
	backups, _ := filepath.Glob(filepath.Join(h, ".config/dots-backups/jcode/*/*"))
	if len(backups) != 2 {
		t.Fatal("missing backups", backups)
	}
	before := tree(t, h)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 || !reflect.DeepEqual(before, tree(t, h)) {
		t.Fatal("not idempotent", code, diagnostic)
	}
}

func TestJcodeApplyPreservesNativeServerKey(t *testing.T) {
	h, root := jcodeFixture(t)
	p := filepath.Join(h, ".jcode/mcp.json")
	put(t, p, `{"servers":{"kaiten":{"command":"personal"},"pilens":{"command":"old"}}}`)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	mcp, err := readObject(p, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicate := mcp["mcpServers"]; duplicate {
		t.Fatal("duplicate server-map aliases make native jcode reject its config")
	}
	servers := mcp["servers"].(map[string]any)
	if len(servers) != 9 || servers["kaiten"].(map[string]any)["command"] != "personal" || servers["context7"].(map[string]any)["command"] != "/fixture/node" || servers["pilens"] != nil {
		t.Fatal("native server-map merge failed")
	}
	before := tree(t, h)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 || !reflect.DeepEqual(before, tree(t, h)) {
		t.Fatal("native server map not idempotent", code, diagnostic)
	}
}

func TestJcodeApplyFreshAndCustomPaths(t *testing.T) {
	h, root := jcodeFixture(t)
	configDir := filepath.Join(h, "config with ' quote")
	dataDir := filepath.Join(h, "data with ' quote")
	t.Setenv("JCODE_CONFIG_DIR", configDir)
	t.Setenv("JCODE_INTEGRATIONS_DIR", dataDir)
	t.Setenv("JCODE_NODE_BIN", "/fixture/node with ' quote")
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(get(t, filepath.Join(configDir, "config.toml"))), &config); err != nil || config["provider"].(map[string]any)["default_provider"] != "openai" {
		t.Fatal("fresh install must explicitly select Codex", err)
	}
	mcp, err := readObject(filepath.Join(configDir, "mcp.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	server := mcp["mcpServers"].(map[string]any)["piweb"].(map[string]any)
	if server["env"].(map[string]any)["PI_CODING_AGENT_DIR"] != filepath.Join(dataDir, "web/config") {
		t.Fatal(server)
	}
}

func TestJcodeInvalidInputNeverPartiallyPublishes(t *testing.T) {
	for _, scenario := range []string{"bad TOML", "bad MCP", "MCP servers array", "duplicate MCP server maps", "missing template", "bad template", "missing voice source", "voice symlink", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			h, root := jcodeFixture(t)
			p := filepath.Join(h, ".jcode/config.toml")
			m := filepath.Join(h, ".jcode/mcp.json")
			put(t, p, "[provider]\ndefault_model='untouched'\n")
			put(t, m, `{"mcpServers":{"personal":{"command":"keep"}}}`)
			switch scenario {
			case "bad TOML":
				put(t, p, "[invalid")
			case "bad MCP":
				put(t, m, `{"token":"fixture-secret"`)
			case "MCP servers array":
				put(t, m, `{"mcpServers":[]}`)
			case "duplicate MCP server maps":
				put(t, m, `{"servers":{},"mcpServers":{}}`)
			case "missing template":
				_ = os.Remove(filepath.Join(root, "config/jcode/mcp.json"))
			case "bad template":
				put(t, filepath.Join(root, "config/jcode/mcp.json"), `[]`)
			case "missing voice source":
				_ = os.Remove(filepath.Join(root, "config/jcode/agent-voice.py"))
			case "voice symlink":
				voice := filepath.Join(h, ".local/share/jcode-integrations/voice/server.py")
				put(t, filepath.Join(h, "outside.py"), "untouched")
				if err := os.MkdirAll(filepath.Dir(voice), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(h, "outside.py"), voice); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				_ = os.Remove(p)
				put(t, filepath.Join(h, "personal.toml"), "personal")
				if e := os.Symlink(filepath.Join(h, "personal.toml"), p); e != nil {
					t.Fatal(e)
				}
			}
			before := tree(t, h)
			code, out, diagnostic := runCaptured(t, root, "jcode-apply")
			if code != 1 || !reflect.DeepEqual(before, tree(t, h)) || strings.Contains(out+diagnostic, "fixture-secret") {
				t.Fatal("partial publication or leaked input", code, out, diagnostic)
			}
		})
	}
}
