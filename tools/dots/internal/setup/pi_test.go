package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func put(t *testing.T, p, s string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
}
func get(t *testing.T, p string) string {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func fixturePi(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "root")
	agent := filepath.Join(base, "agent")
	t.Setenv("HOME", base)
	t.Setenv("PI_CODING_AGENT_DIR", agent)
	put(t, filepath.Join(root, "config/pi/settings.json"), `{"theme":"ayu","defaultProvider":"openai","defaultModel":"gpt-6.1-sol","modelThinkingLevels":{"openai/gpt-6.1-sol":"xhigh"},"httpProxy":"http://127.0.0.1:10810","terminal":{"showTerminalProgress":true},"hideThinkingBlock":true,"enableInstallTelemetry":false,"defaultProjectTrust":"always"}`)
	put(t, filepath.Join(root, "config/pi/models.json"), `{"providers":{"openai":{"modelOverrides":{"gpt-6.1-sol":{"contextWindow":400000}}}}}`)
	put(t, filepath.Join(root, "config/pi/mcp.json"), `{"mcpServers":{"excalidraw":{"type":"http","url":"https://mcp.excalidraw.com/mcp","timeout":60,"exposure":"codemode"},"peekaboo":{"command":"/opt/homebrew/bin/peekaboo","args":["mcp","--allow-foreground"],"exposure":"codemode","toolExposure":{"agent":"hidden","analyze":"hidden","browser":"hidden"}}}}`)
	for _, r := range []string{"themes/ayu.json", "extensions/ayu-bar/index.ts", "extensions/task-titles/index.ts"} {
		put(t, filepath.Join(root, "config/pi", r), "resource")
	}
	return root, agent
}
func TestPiPreservesLocalData(t *testing.T) {
	root, agent := fixturePi(t)
	put(t, filepath.Join(agent, "settings.json"), `{"theme":"dark","httpProxy":"http://127.0.0.1:10810","modelThinkingLevels":{"local/other":"low"},"terminal":{"showImages":false},"extensions":["other.ts"],"deviceId":"local-device","lastChangelogVersion":"local-version"}`)
	put(t, filepath.Join(agent, "models.json"), `{"providers":{"openai":{"headers":{"X-Test":"keep"},"modelOverrides":{"gpt-6.1-sol":{"maxTokens":16384},"other-model":{"contextWindow":123456}}},"local":{"baseUrl":"http://localhost:1234/v1"}}}`)
	auth := filepath.Join(agent, "auth.json")
	put(t, auth, "fixture-auth-untouched")
	put(t, filepath.Join(agent, "extensions/other.ts"), "keep me")
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	settings, e := readObject(filepath.Join(agent, "settings.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	for k, v := range map[string]any{"theme": "ayu", "deviceId": "local-device", "httpProxy": "http://127.0.0.1:10810", "lastChangelogVersion": "local-version"} {
		if settings[k] != v {
			t.Errorf("%s: %v", k, settings[k])
		}
	}
	if !reflect.DeepEqual(settings["terminal"], map[string]any{"showImages": false, "showTerminalProgress": true}) {
		t.Fatal(settings)
	}
	models := get(t, filepath.Join(agent, "models.json"))
	for _, s := range []string{"400000", "16384", "123456", "X-Test", "localhost"} {
		if !strings.Contains(models, s) {
			t.Fatal(models)
		}
	}
	if get(t, auth) != "fixture-auth-untouched" || get(t, filepath.Join(agent, "extensions/other.ts")) != "keep me" {
		t.Fatal("local data changed")
	}
	for _, name := range []string{"themes/ayu.json", "extensions/dots-ayu-bar", "extensions/dots-task-titles"} {
		if info, e := os.Lstat(filepath.Join(agent, name)); e != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal(name, e)
		}
	}
	backups, _ := filepath.Glob(filepath.Join(agent, "settings.json.bak-*"))
	if len(backups) != 1 || !strings.Contains(get(t, backups[0]), `"dark"`) {
		t.Fatal(backups)
	}
}

func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	e := filepath.Walk(root, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		if i.Mode()&os.ModeSymlink != 0 {
			v, e := os.Readlink(p)
			if e != nil {
				return e
			}
			out[rel] = "link:" + v
		} else if !i.IsDir() {
			out[rel] = get(t, p)
		} else {
			out[rel] = "dir"
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestPiIdempotentAndPrivate(t *testing.T) {
	root, agent := fixturePi(t)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	before := tree(t, agent)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("repeat")
	}
	if !reflect.DeepEqual(before, tree(t, agent)) {
		t.Fatal("repeat changed files")
	}
	for _, name := range []string{"settings.json", "models.json", "mcp.json"} {
		i, e := os.Stat(filepath.Join(agent, name))
		if e != nil || i.Mode().Perm() != 0600 {
			t.Fatal("mode", e)
		}
	}
}
func TestPiPersonalPreferences(t *testing.T) {
	root, agent := fixturePi(t)
	put(t, filepath.Join(agent, "settings.json"), `{"defaultProvider":"local","defaultModel":"other","modelThinkingLevels":{"openai/gpt-6.1-sol":"low"},"hideThinkingBlock":false,"enableInstallTelemetry":true,"quietStartup":true}`)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	expected, e := readObject(filepath.Join(root, "config/pi/settings.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	expected["quietStartup"] = true
	actual, e := readObject(filepath.Join(agent, "settings.json"), false)
	if e != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatal(actual, e)
	}
}
func TestPiResourceBackupsOutsideDiscovery(t *testing.T) {
	root, agent := fixturePi(t)
	put(t, filepath.Join(agent, "themes/ayu.json"), "old theme")
	put(t, filepath.Join(agent, "extensions/dots-ayu-bar/index.ts"), "old extension")
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	for name, want := range map[string]string{"ayu.json": "old theme", "dots-ayu-bar": "old extension"} {
		files, _ := filepath.Glob(filepath.Join(agent, "backups", name+".bak-*"))
		if len(files) != 1 {
			t.Fatal(files)
		}
		p := files[0]
		if name == "dots-ayu-bar" {
			p = filepath.Join(p, "index.ts")
		}
		if get(t, p) != want {
			t.Fatal("backup content")
		}
	}
	entries, e := os.ReadDir(filepath.Join(agent, "extensions"))
	if e != nil || len(entries) != 2 {
		t.Fatal(entries, e)
	}
}
func TestPiSettingsSymlink(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "dangling"}[dangling], func(t *testing.T) {
			root, agent := fixturePi(t)
			target := filepath.Join(filepath.Dir(agent), "personal-settings.json")
			if !dangling {
				put(t, target, `{"theme":"dark","quietStartup":true}`)
				if e := os.Chmod(target, 0640); e != nil {
					t.Fatal(e)
				}
			}
			if e := os.MkdirAll(agent, 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink("../personal-settings.json", filepath.Join(agent, "settings.json")); e != nil {
				t.Fatal(e)
			}
			if Run(root, []string{"pi-apply"}) != 0 {
				t.Fatal("apply")
			}
			i, e := os.Lstat(filepath.Join(agent, "settings.json"))
			if e != nil || i.Mode()&os.ModeSymlink == 0 {
				t.Fatal("symlink replaced")
			}
			data, e := readObject(target, false)
			if e != nil || data["theme"] != "ayu" {
				t.Fatal(data, e)
			}
			if !dangling {
				if data["quietStartup"] != true {
					t.Fatal(data)
				}
				files, _ := filepath.Glob(target + ".bak-*")
				if len(files) != 1 {
					t.Fatal(files)
				}
				i, _ := os.Stat(target)
				if i.Mode().Perm() != 0640 {
					t.Fatal("mode changed")
				}
			}
		})
	}
}
func TestPiInvalidNoMutation(t *testing.T) {
	for _, name := range []string{"settings.json", "models.json", "mcp.json"} {
		for _, data := range []string{"bad JSON", "", "[]", "null", "{} {}"} {
			t.Run(name+data, func(t *testing.T) {
				root, agent := fixturePi(t)
				put(t, filepath.Join(agent, name), data)
				before := tree(t, agent)
				if Run(root, []string{"pi-apply"}) != 1 {
					t.Fatal("accepted malformed")
				}
				if !reflect.DeepEqual(tree(t, agent), before) {
					t.Fatal("mutation")
				}
			})
		}
	}
}
func TestPiMCPPreservesLocalServers(t *testing.T) {
	root, agent := fixturePi(t)
	path := filepath.Join(agent, "mcp.json")
	original := `{"autoEnableCodemode":false,"mcpServers":{"personal":{"url":"https://example.invalid/mcp","headers":{"Authorization":"fixture-private-token"}},"excalidraw":{"type":"stdio","command":"old-excalidraw","args":["--stdio"],"env":{"TOKEN":"old-token"},"enabled":false},"peekaboo":{"url":"https://old.invalid/mcp","headers":{"Authorization":"old-token"},"enabled":false}}}`
	put(t, path, original)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	current, err := readObject(path, false)
	if err != nil {
		t.Fatal(err)
	}
	servers := current["mcpServers"].(map[string]any)
	want, _ := object([]byte(original))
	if !reflect.DeepEqual(servers["personal"], want["mcpServers"].(map[string]any)["personal"]) || current["autoEnableCodemode"] != false {
		t.Fatal("local MCP data changed")
	}
	template, _ := readObject(filepath.Join(root, "config/pi/mcp.json"), false)
	for _, name := range []string{"excalidraw", "peekaboo"} {
		if !reflect.DeepEqual(servers[name], template["mcpServers"].(map[string]any)[name]) {
			t.Fatal(name, "managed server retained stale transport, credentials or enabled state")
		}
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 1 || get(t, backups[0]) != original {
		t.Fatal("missing original MCP backup")
	}
	before := tree(t, agent)
	if Run(root, []string{"pi-apply"}) != 0 || !reflect.DeepEqual(before, tree(t, agent)) {
		t.Fatal("MCP apply is not idempotent")
	}
	if strings.Contains(get(t, filepath.Join(root, "config/pi/mcp.json")), "fixture-private-token") {
		t.Fatal("private MCP credentials leaked into dots")
	}
}

func TestPiInvalidMCPServersNoMutation(t *testing.T) {
	for _, location := range []string{"live", "template"} {
		for _, data := range []string{`{"mcpServers":null}`, `{"mcpServers":[]}`, `{"mcpServers":"invalid"}`} {
			t.Run(location+data, func(t *testing.T) {
				root, agent := fixturePi(t)
				put(t, filepath.Join(agent, "settings.json"), `{"theme":"old"}`)
				path := filepath.Join(agent, "mcp.json")
				if location == "template" {
					path = filepath.Join(root, "config/pi/mcp.json")
				}
				put(t, path, data)
				before := tree(t, agent)
				if Run(root, []string{"pi-apply"}) != 1 || !reflect.DeepEqual(before, tree(t, agent)) {
					t.Fatal("invalid MCP object accepted or live files changed")
				}
			})
		}
	}
}

func TestPiMissingResourceNoMutation(t *testing.T) {
	root, agent := fixturePi(t)
	put(t, filepath.Join(agent, "settings.json"), `{"theme":"old"}`)
	if e := os.RemoveAll(filepath.Join(root, "config/pi/extensions/task-titles")); e != nil {
		t.Fatal(e)
	}
	before := tree(t, agent)
	if Run(root, []string{"pi-apply"}) != 1 {
		t.Fatal("accepted missing resource")
	}
	if !reflect.DeepEqual(before, tree(t, agent)) {
		t.Fatal("mutation")
	}
}
