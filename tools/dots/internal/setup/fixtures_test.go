package setup

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	switch filepath.Base(os.Args[0]) {
	case "codex":
		os.Exit(fakeCodex())
	case "claude":
		os.Exit(fakeClaude())
	case "defaults":
		os.Exit(fakeDefaults())
	case "npm":
		fmt.Fprintln(os.Stderr, "fixture-secret-install-error")
		os.Exit(17)
	}
	os.Exit(m.Run())
}
func fakeCommand(t *testing.T, name string) {
	t.Helper()
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	bin := filepath.Join(home(), "bin")
	if e = os.MkdirAll(bin, 0700); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(bin, name)
	if e = os.Symlink(binary, p); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin)
}
func fakeClaude() int {
	b, _ := json.Marshal(os.Args[1:])
	f, e := os.OpenFile(filepath.Join(home(), "claude-argv.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return 99
	}
	defer f.Close()
	f.Write(append(b, '\n'))
	if os.Getenv("FIXTURE_CLAUDE_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "fixture-secret-error-output")
		return 7
	}
	return 0
}
func fakeCodex() int {
	if len(os.Args) != 2 || os.Args[1] != "app-server" {
		return 99
	}
	dir := os.Getenv("CODEX_HOME")
	scan := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scan.Scan() {
		var request map[string]any
		if json.Unmarshal(scan.Bytes(), &request) != nil {
			return 98
		}
		method, _ := request["method"].(string)
		id := request["id"]
		switch method {
		case "initialize":
			if nested(request, "params", "clientInfo")["name"] != "dots_abide" {
				return 97
			}
			encoder.Encode(map[string]any{"id": id, "result": map[string]any{}})
		case "initialized":
			continue
		case "hooks/list":
			if os.Getenv("FIXTURE_CODEX_ERROR") == "1" {
				encoder.Encode(map[string]any{"id": id, "error": map[string]any{"message": "fixture-secret-protocol"}})
				continue
			}
			countPath := filepath.Join(dir, "fixture-count")
			b, _ := os.ReadFile(countPath)
			count := len(b) + 1
			os.WriteFile(countPath, append(b, 'x'), 0600)
			cfgb, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
			cfg, e := parseTOML(string(cfgb))
			if e != nil {
				return 96
			}
			hooksb, _ := os.ReadFile(filepath.Join(dir, "hooks.json"))
			data := map[string]any{}
			if len(hooksb) > 0 {
				data, e = object(hooksb)
				if e != nil {
					return 95
				}
			}
			var entries []hookEntry
			if nested(cfg, "features")["hooks"] == true {
				for _, event := range []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop"} {
					_, groups, e := hookGroups(data, event)
					if e != nil {
						return 94
					}
					for _, g := range groups {
						_, hooks, e := groupEntries(g)
						if e != nil {
							return 93
						}
						for _, v := range hooks {
							entry := v.(map[string]any)
							command, _ := entry["command"].(string)
							hash := fmt.Sprintf("%x", sha256.Sum256(encode(entry)))
							key := fmt.Sprintf("global:%x", sha256.Sum256([]byte(event+command)))
							source := filepath.Join(dir, "hooks.json")
							state := nested(cfg, "hooks", "state", key)
							trust := "untrusted"
							if state["trusted_hash"] == hash {
								trust = "trusted"
							}
							enabled := state["enabled"] != false
							h := hookEntry{key, source, command, hash, trust, enabled}
							if os.Getenv("FIXTURE_OTHER_CHANGE") == "1" && count >= 2 && command == "unrelated" {
								h.CurrentHash = "unexpected"
							}
							if os.Getenv("FIXTURE_SOURCE_MISMATCH") == "1" && isManagedTask(command) {
								h.SourcePath = filepath.Join(dir, "other-hooks.json")
							}
							entries = append(entries, h)
						}
					}
				}
			}
			encoder.Encode(map[string]any{"method": "notification", "params": map[string]any{}})
			encoder.Encode(map[string]any{"id": id, "result": map[string]any{"data": []any{map[string]any{"errors": []any{}, "hooks": entries}}}})
		default:
			return 92
		}
	}
	return 0
}
func fixtureClaude(t *testing.T) (string, string, string) {
	t.Helper()
	h := fixtureHome(t)
	root := filepath.Join(h, "root")
	src := filepath.Join(root, "config/claude")
	dir := filepath.Join(h, ".claude")
	put(t, filepath.Join(src, "settings.json"), `{"env":{"ANTHROPIC_BASE_URL":"${ANTHROPIC_BASE_URL}"},"permissions":{"allow":[]},"hooks":{"Other":"keep"}}`)
	put(t, filepath.Join(src, "omc-config.json"), `{"theme":"dark"}`)
	put(t, filepath.Join(src, "mcp-servers.json"), `{"zai-mcp-server":{"command":"app","env":{"Z_AI_API_KEY":"${Z_AI_API_KEY}"}},"safe":{"command":"app","args":["literal"]},"blocked":{"env":{"BAD":"${NOT_WHITELISTED}"}}}`)
	return root, src, dir
}
func TestClaudeApplyWhitelistAndRedaction(t *testing.T) {
	root, _, dir := fixtureClaude(t)
	fakeCommand(t, "claude")
	t.Setenv("HAVE_CLAUDE", "1")
	t.Setenv("Z_AI_API_KEY", "fixture-secret-key")
	t.Setenv("ANTHROPIC_BASE_URL", "http://fixture.invalid")
	t.Setenv("NOT_WHITELISTED", "must-not-be-used")
	t.Setenv("FIXTURE_CLAUDE_FAIL", "1")
	put(t, filepath.Join(dir, "auth.json"), "unread-auth")
	code, out, err := runCaptured(t, root, "claude-apply-json")
	if code != 0 {
		t.Fatal(code, out, err)
	}
	for _, v := range []string{"fixture-secret-key", "fixture-secret-error-output", "must-not-be-used"} {
		if strings.Contains(out+err, v) {
			t.Fatal("secret leaked")
		}
	}
	if !strings.Contains(out, "NOT_WHITELISTED") || !strings.Contains(out, "ошибка регистрации") {
		t.Fatal(out)
	}
	if get(t, filepath.Join(dir, "auth.json")) != "unread-auth" {
		t.Fatal("auth modified")
	}
	settings, e := readObject(filepath.Join(dir, "settings.json"), false)
	if e != nil || nested(settings, "env")["ANTHROPIC_BASE_URL"] != "http://fixture.invalid" {
		t.Fatal(settings, e)
	}
	argv := get(t, filepath.Join(home(), "claude-argv.jsonl"))
	if !strings.Contains(argv, "fixture-secret-key") || strings.Contains(argv, "must-not-be-used") {
		t.Fatal("incorrect substitution")
	}
}
func TestClaudeApplyMissingEnvAndMalformedNoMutation(t *testing.T) {
	root, src, dir := fixtureClaude(t)
	code, out, err := runCaptured(t, root, "claude-apply-json")
	if code != 0 || !strings.Contains(out, "регистрация MCP пропущена") {
		t.Fatal(code, out, err)
	}
	d, _ := readObject(filepath.Join(dir, "settings.json"), false)
	if _, ok := nested(d, "env")["ANTHROPIC_BASE_URL"]; ok {
		t.Fatal("missing env not removed")
	}
	before := tree(t, dir)
	put(t, filepath.Join(src, "mcp-servers.json"), "fixture-secret-bad-json")
	code, out, err = runCaptured(t, root, "claude-apply-json")
	if code != 1 || strings.Contains(out+err, "fixture-secret") {
		t.Fatal(code, out, err)
	}
	after := tree(t, dir)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatal("malformed input modified files")
	}
}
func TestClaudeSyncMaskAndLeakNoMutation(t *testing.T) {
	root, src, dir := fixtureClaude(t)
	put(t, filepath.Join(dir, "settings.json"), `{"permissions":{"allow":["safe","/Users/person/project"]},"env":{"ANTHROPIC_BASE_URL":"http://private-endpoint"},"path":"`+home()+`/hook"}`)
	put(t, filepath.Join(dir, ".omc-config.json"), `{"keep":true}`)
	put(t, filepath.Join(home(), ".claude.json"), `{"mcpServers":{"magic":{"env":{"API_KEY":"fixture-secret","CUSTOM_TOKEN":"fixture-secret-2"},"args":["/Users/person/config.yaml","other"]}}}`)
	code, out, err := runCaptured(t, root, "claude-sync-json")
	if code != 0 || strings.Contains(out+err, "fixture-secret") {
		t.Fatal(code, out, err)
	}
	b := get(t, filepath.Join(src, "mcp-servers.json"))
	for _, s := range []string{"${MAGIC_API_KEY}", "${CUSTOM_TOKEN}", "${KUBECONFIG_PATH}"} {
		if !strings.Contains(b, s) {
			t.Fatal(b)
		}
	}
	if strings.Contains(b, "fixture-secret") {
		t.Fatal("secret exported")
	}
	b = get(t, filepath.Join(src, "settings.json"))
	if !strings.Contains(b, "$HOME/hook") || strings.Contains(b, "/Users/person") || strings.Contains(b, "private-endpoint") {
		t.Fatal(b)
	}
	before := tree(t, src)
	put(t, filepath.Join(dir, "settings.json"), `{"permissions":{"allow":[]},"env":{"API_TOKEN":"fixture-leak"}}`)
	code, out, err = runCaptured(t, root, "claude-sync-json")
	if code != 1 || strings.Contains(out+err, "fixture-leak") {
		t.Fatal(code, out, err)
	}
	if fmt.Sprint(before) != fmt.Sprint(tree(t, src)) {
		t.Fatal("leak check published files")
	}
}
