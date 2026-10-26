package setup

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestJcodeTemplatePinsNeo4jBackend(t *testing.T) {
	template, err := readObject(filepath.Join("..", "..", "..", "..", "config", "jcode", "mcp.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	server := template["mcpServers"].(map[string]any)["code-graph"].(map[string]any)
	env := server["env"].(map[string]any)
	if env["DEFAULT_DATABASE"] != "neo4j" || env["NEO4J_DATABASE"] != "neo4j" || env["DEFAULT_BACKEND"] != nil {
		t.Fatal("CodeGraphContext must explicitly select the supported Neo4j backend and database")
	}
	if !reflect.DeepEqual(server["args"], []any{"--database", "neo4j", "mcp", "start"}) {
		t.Fatal("CLI backend must override inherited CGC_RUNTIME_DB_TYPE")
	}
}

func TestJcodeCodexOnlyAndManagedServers(t *testing.T) {
	h, root := jcodeFixture(t)
	p := filepath.Join(h, ".jcode/config.toml")
	put(t, p, `[provider]
default_provider='openrouter'
default_model='my-codex-model'
openai_reasoning_effort='xhigh'
cross_provider_failover='auto'
[features]
swarm=true
auto_poke=true
memory=true
[agents]
memory_sidecar_enabled=true
swarm_max_concurrent_agents=32
[hooks]
session_end='keep'
`)
	put(t, filepath.Join(h, ".jcode/mcp.json"), `{"servers":{"kaiten":{"command":"keep"},"peekaboo":{"command":"keep"},"pilens":{"command":"old"}}}`)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(get(t, p)), &config); err != nil {
		t.Fatal(err)
	}
	provider := config["provider"].(map[string]any)
	if provider["default_provider"] != "openai" || provider["default_model"] != "my-codex-model" || provider["openai_reasoning_effort"] != "xhigh" || provider["cross_provider_failover"] != "manual" {
		t.Fatal("Codex-only provider/model contract broken")
	}
	for _, name := range []string{"swarm", "auto_poke", "memory"} {
		if config["features"].(map[string]any)[name] != false {
			t.Fatal("background feature enabled:", name)
		}
	}
	agents := config["agents"].(map[string]any)
	if agents["memory_sidecar_enabled"] != false || agents["swarm_max_concurrent_agents"] != int64(1) || config["hooks"].(map[string]any)["session_end"] != "keep" {
		t.Fatal("background agents enabled or personal hooks overwritten")
	}
	mcp, err := readObject(filepath.Join(h, ".jcode/mcp.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	servers := mcp["servers"].(map[string]any)
	if len(servers) != 10 || servers["pilens"] != nil {
		t.Fatal("wrong managed server set")
	}
	for _, name := range []string{"serena", "context7", "deepwiki", "code-graph", "neo4j-cypher", "neo4j-memory", "piweb", "agent-voice", "kaiten", "peekaboo"} {
		if servers[name] == nil {
			t.Fatal("missing server:", name)
		}
	}
}

func TestJcodeNeo4jPasswordPrivateStableAndRejectsInvalidPaths(t *testing.T) {
	t.Run("generated and reused", func(t *testing.T) {
		h, root := jcodeFixture(t)
		p := filepath.Join(h, ".local/share/jcode-integrations/neo4j/private.env")
		if code, out, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
			t.Fatal(code, out, diagnostic)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal("missing private Neo4j credentials:", err)
		}
		value := strings.TrimSuffix(strings.TrimPrefix(string(data), "NEO4J_PASSWORD="), "\n")
		if len(value) != 64 {
			t.Fatal("password is not 256 bits")
		}
		if _, err := hex.DecodeString(value); err != nil {
			t.Fatal("password is not hex")
		}
		for _, path := range []string{p, filepath.Dir(p)} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			want := os.FileMode(0600)
			if info.IsDir() {
				want = 0700
			}
			if info.Mode().Perm() != want {
				t.Fatal("credentials not private")
			}
		}
		before := tree(t, h)
		if code, out, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 || !reflect.DeepEqual(before, tree(t, h)) || strings.Contains(out+diagnostic, value) {
			t.Fatal("credentials changed, leaked, or rerun not idempotent")
		}
		mcp, _ := readObject(filepath.Join(h, ".jcode/mcp.json"), false)
		servers := mcp["mcpServers"].(map[string]any)
		for _, name := range []string{"code-graph", "neo4j-cypher", "neo4j-memory"} {
			if servers[name].(map[string]any)["env"].(map[string]any)["NEO4J_PASSWORD"] != value {
				t.Fatal("database tools do not share the private credential")
			}
		}
	})
	for _, scenario := range []string{"malformed", "public", "symlink file", "symlink directory"} {
		t.Run(scenario, func(t *testing.T) {
			h, root := jcodeFixture(t)
			p := filepath.Join(h, ".local/share/jcode-integrations/neo4j/private.env")
			put(t, p, "NEO4J_PASSWORD="+strings.Repeat("a", 64)+"\n")
			switch scenario {
			case "malformed":
				put(t, p, "NEO4J_PASSWORD=fixture-secret\n")
			case "public":
				if err := os.Chmod(p, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink file":
				_ = os.Remove(p)
				put(t, filepath.Join(h, "outside"), "fixture-secret")
				if err := os.Symlink(filepath.Join(h, "outside"), p); err != nil {
					t.Fatal(err)
				}
			case "symlink directory":
				if err := os.Rename(filepath.Dir(p), filepath.Join(h, "outside")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(h, "outside"), filepath.Dir(p)); err != nil {
					t.Fatal(err)
				}
			}
			before := tree(t, h)
			code, out, diagnostic := runCaptured(t, root, "jcode-apply")
			if code != 1 || !reflect.DeepEqual(before, tree(t, h)) || strings.Contains(out+diagnostic, "fixture-secret") {
				t.Fatal("unsafe credential accepted, changed, or exposed")
			}
		})
	}
}
