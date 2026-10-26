package setup

import (
	"errors"
	"os"
	"os/exec"
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
	put(t, filepath.Join(root, "config/jcode/swarm-model-gate.py"), "print('fixture gate')\n")
	put(t, filepath.Join(root, "config/jcode/theme.toml"), "[display]\ntheme = 'dark'\n[display.colors]\naccent = '#73d0ff'\n")
	put(t, filepath.Join(root, "config/jcode/config.toml"), "[provider]\ndefault_provider = 'openai'\ndefault_model = 'gpt-6.1-sol'\nopenai_reasoning_effort = 'high'\ncross_provider_failover = 'manual'\n[features]\nswarm = true\nauto_poke = false\nmemory = false\n[agents]\nswarm_model = 'openai-oauth:gpt-6.1-sol'\nswarm_effort = 'low'\nswarm_root_effort = 'high'\nswarm_deep_root_effort = 'high'\nswarm_max_concurrent_agents = 8\nmemory_sidecar_enabled = false\n")
	put(t, filepath.Join(root, "config/jcode/swarm-routing"), "previous routing policy\n")
	put(t, filepath.Join(root, "config/jcode/swarm-routing-next"), "managed routing policy\n")
	return h, root
}

func TestJcodeTrackedModelRouting(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "config", "jcode")
	var config map[string]any
	if err := toml.Unmarshal([]byte(get(t, filepath.Join(root, "config.toml"))), &config); err != nil {
		t.Fatal(err)
	}
	provider, agents, features := config["provider"].(map[string]any), config["agents"].(map[string]any), config["features"].(map[string]any)
	if provider["default_provider"] != "openai" || provider["default_model"] != "gpt-6.1-sol" || provider["openai_reasoning_effort"] != "high" || agents["swarm_model"] != "openai-oauth:gpt-6.1-sol" || agents["swarm_effort"] != "low" || agents["swarm_root_effort"] != "high" || agents["swarm_deep_root_effort"] != "high" || features["swarm"] != true {
		t.Fatal("tracked defaults do not match the model routing policy")
	}
	if _, exists := config["providers"]; exists {
		t.Fatal("shared settings contain private provider profiles")
	}
	policy := get(t, filepath.Join(root, "swarm-routing-next"))
	for _, route := range []string{"x5-copilot:x5-airun-code-large", "openai-oauth:gpt-6.1-terra", "openai-oauth:gpt-6.1-sol", `effort "max"`, `effort "low"`} {
		if !strings.Contains(policy, route) {
			t.Fatalf("routing policy missing %q", route)
		}
	}
}

func TestJcodeApplyAddsSwarmGateWithoutReplacingPersonalHook(t *testing.T) {
	h, root := jcodeFixture(t)
	configPath := filepath.Join(h, ".jcode/config.toml")
	put(t, configPath, "[hooks]\npre_tool = 'personal-hook'\n")
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(get(t, configPath)), &config); err != nil {
		t.Fatal(err)
	}
	hooks, ok := config["hooks"].(map[string]any)["pre_tool"].([]any)
	if !ok || len(hooks) != 2 || hooks[0] != "personal-hook" || !strings.Contains(hooks[1].(string), "swarm-model-gate.py") {
		t.Fatalf("personal hook or swarm gate missing: %#v", hooks)
	}
	if get(t, filepath.Join(h, ".local/share/jcode-integrations/swarm-model-gate.py")) != "print('fixture gate')\n" {
		t.Fatal("swarm gate was not installed")
	}
	before := tree(t, h)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 || !reflect.DeepEqual(before, tree(t, h)) {
		t.Fatal("reapplying must not duplicate the hook", code, diagnostic)
	}
}

func TestJcodeApplyUpdatesManagedGateInterpreter(t *testing.T) {
	h, root := jcodeFixture(t)
	configPath := filepath.Join(h, ".jcode/config.toml")
	gatePath := filepath.Join(h, ".local/share/jcode-integrations/swarm-model-gate.py")
	put(t, configPath, "[hooks]\npre_tool = [\"personal-hook\", \"audit-wrapper '"+gatePath+"'\"]\n")
	first, second := filepath.Join(h, "first"), filepath.Join(h, "second")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "python3"), []byte("#!/bin/sh\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", first)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	t.Setenv("PATH", second)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(get(t, configPath)), &config); err != nil {
		t.Fatal(err)
	}
	hooks := config["hooks"].(map[string]any)["pre_tool"].([]any)
	if len(hooks) != 3 || hooks[0] != "personal-hook" || hooks[1] != "audit-wrapper '"+gatePath+"'" || !strings.Contains(hooks[2].(string), second) {
		t.Fatalf("managed gate must use the current interpreter exactly once: %#v", hooks)
	}
}

func TestJcodeInstalledGateRunsFromQuotedPath(t *testing.T) {
	h, root := jcodeFixture(t)
	dataDir := filepath.Join(h, "data with ' quote")
	t.Setenv("JCODE_INTEGRATIONS_DIR", dataDir)
	source := get(t, filepath.Join("..", "..", "..", "..", "config", "jcode", "swarm-model-gate.py"))
	put(t, filepath.Join(root, "config/jcode/swarm-model-gate.py"), source)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(get(t, filepath.Join(h, ".jcode/config.toml"))), &config); err != nil {
		t.Fatal(err)
	}
	hooks := config["hooks"].(map[string]any)["pre_tool"].([]any)
	command := exec.Command("/bin/sh", "-c", hooks[0].(string))
	command.Env = append(os.Environ(), "JCODE_HOOK_TOOL_NAME=swarm")
	command.Stdin = strings.NewReader(`{"action":"spawn","label":"reviewer","model":"inherit"}`)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 || !strings.Contains(string(output), "gpt-6.1-sol") {
		t.Fatalf("installed gate did not reject inherited reviewer: %v %s", err, output)
	}
}

func TestJcodeApplyPreservesSettingsAndIdempotence(t *testing.T) {
	h, root := jcodeFixture(t)
	configPath := filepath.Join(h, ".jcode/config.toml")
	mcpPath := filepath.Join(h, ".jcode/mcp.json")
	put(t, configPath, "[provider]\ndefault_model = 'personal-model'\n[providers.private]\nbase_url = 'https://private.example/v1'\napi_key_env = 'PRIVATE_KEY'\n[hooks]\nsession_end = 'personal-hook'\n[launch_hotkeys]\nenabled = true\n")
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
	if config["provider"].(map[string]any)["default_model"] != "gpt-6.1-sol" || config["hooks"].(map[string]any)["session_end"] != "personal-hook" || config["launch_hotkeys"].(map[string]any)["enabled"] != true {
		t.Fatal(config)
	}
	private := config["providers"].(map[string]any)["private"].(map[string]any)
	if private["base_url"] != "https://private.example/v1" || private["api_key_env"] != "PRIVATE_KEY" {
		t.Fatal("private provider changed")
	}
	if config["features"].(map[string]any)["swarm"] != true || config["agents"].(map[string]any)["swarm_model"] != "openai-oauth:gpt-6.1-sol" || config["agents"].(map[string]any)["swarm_max_concurrent_agents"] != int64(8) {
		t.Fatal("managed swarm settings not applied")
	}
	policy := filepath.Join(h, ".jcode/swarm-prompt.md")
	if get(t, policy) != "managed routing policy\n" {
		t.Fatal("routing policy not installed")
	}
	link, err := os.Readlink(policy)
	if err != nil || link != filepath.Join(root, "config/jcode/swarm-routing-next") {
		t.Fatal("routing link must select the new policy", link, err)
	}
	if get(t, filepath.Join(root, "config/jcode/swarm-routing")) != "previous routing policy\n" {
		t.Fatal("previous routing policy must remain untouched")
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

func TestJcodeApplyBacksUpExistingSwarmPrompt(t *testing.T) {
	h, root := jcodeFixture(t)
	path := filepath.Join(h, ".jcode/swarm-prompt.md")
	put(t, path, "personal routing\n")
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	if get(t, path) != "managed routing policy\n" {
		t.Fatal("shared policy not installed")
	}
	backups, err := filepath.Glob(filepath.Join(h, ".config/dots-backups/jcode/*/swarm-prompt.md"))
	if err != nil || len(backups) != 1 || get(t, backups[0]) != "personal routing\n" {
		t.Fatal("original policy was not backed up")
	}
}

func TestJcodeApplyRelativeRootCreatesUsablePolicyLink(t *testing.T) {
	h, root := jcodeFixture(t)
	t.Chdir(root)
	if code, _, diagnostic := runCaptured(t, ".", "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	if get(t, filepath.Join(h, ".jcode/swarm-prompt.md")) != "managed routing policy\n" {
		t.Fatal("routing link must resolve independently of the working directory")
	}
}

func TestJcodeRejectsPrivateProviderInSharedTemplate(t *testing.T) {
	h, root := jcodeFixture(t)
	path := filepath.Join(h, ".jcode/config.toml")
	put(t, path, "[providers.personal]\nbase_url='https://private.example/v1'\n")
	put(t, filepath.Join(root, "config/jcode/config.toml"), "[providers.bad]\nbase_url='https://private.example/v1'\n")
	if code, _, _ := runCaptured(t, root, "jcode-apply"); code == 0 {
		t.Fatal("private providers must not be shared")
	}
	if get(t, path) != "[providers.personal]\nbase_url='https://private.example/v1'\n" {
		t.Fatal("rejected install modified the private config")
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
