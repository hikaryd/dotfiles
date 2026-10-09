package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTaskTitleProtocolTrustAndPreservation(t *testing.T) {
	h := fixtureHome(t)
	fakeCommand(t, "codex")
	dir := filepath.Join(h, "custom-codex")
	t.Setenv("CODEX_CONFIG_DIR", dir)
	hp, cp := filepath.Join(dir, "hooks.json"), filepath.Join(dir, "config.toml")
	put(t, hp, `{"other":"keep","hooks":{"UserPromptSubmit":[{"matcher":"custom","metadata":"keep","hooks":[{"type":"command","command":"unrelated","timeout":3},{"type":"command","command":"old hook # dots-task-title","timeout":1}]}]}}`)
	put(t, cp, "# keep comment\n[features]\nother = true\n[hooks.state.\"unrelated-state\"]\ntrusted_hash = \"old\"\nenabled = false\n")
	code, out, err := runCaptured(t, h, "codex-task-title-apply")
	if code != 0 {
		t.Fatal(code, out, err)
	}
	d, e := readObject(hp, false)
	if e != nil {
		t.Fatal(e)
	}
	groups := nested(d, "hooks")["UserPromptSubmit"].([]any)
	g := groups[0].(map[string]any)
	if len(groups) != 1 || g["matcher"] != "custom" || g["metadata"] != "keep" {
		t.Fatal(d)
	}
	entries := g["hooks"].([]any)
	if entries[0].(map[string]any)["command"] != "unrelated" {
		t.Fatal(d)
	}
	command, _ := taskCommand()
	if !strings.HasSuffix(command, " agents codex-task-title hook # dots-task-title") {
		t.Fatalf("hook targets a missing multicall command: %s", command)
	}
	if entries[1].(map[string]any)["command"] != command {
		t.Fatal(d)
	}
	cfg := get(t, cp)
	if !strings.Contains(cfg, "# keep comment") || !strings.Contains(cfg, "other = true") {
		t.Fatal(cfg)
	}
	parsed, e := parseTOML(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if nested(parsed, "hooks", "state", "unrelated-state")["enabled"] != false {
		t.Fatal(parsed)
	}
	b := get(t, hp)
	code, out, err = runCaptured(t, h, "codex-task-title-apply")
	if code != 0 || get(t, hp) != b || get(t, cp) != cfg {
		t.Fatal(code, out, err, "idempotence")
	}
	backupRoot := filepath.Join(h, ".config/dots-backups/task-titles")
	if _, e := os.Stat(backupRoot); e != nil {
		t.Fatal(e)
	}
}
func TestTaskTitleRejectsDuplicateAndMalformed(t *testing.T) {
	for _, data := range []string{`{"hooks":{"UserPromptSubmit":[{"hooks":[{"command":"one # dots-task-title"},{"command":"two # dots-task-title"}]}]}}`, "", `{"hooks":[]}`} {
		t.Run(data, func(t *testing.T) {
			h := fixtureHome(t)
			fakeCommand(t, "codex")
			dir := filepath.Join(h, ".codex")
			put(t, filepath.Join(dir, "hooks.json"), data)
			put(t, filepath.Join(dir, "config.toml"), "# original\n")
			before := tree(t, dir)
			code, _, _ := runCaptured(t, h, "codex-task-title-apply")
			if code != 1 || !reflect.DeepEqual(before, tree(t, dir)) {
				t.Fatal("mutated invalid input")
			}
		})
	}
}
func TestTaskTitleTrustRefusedOnChangedOtherOrSource(t *testing.T) {
	for _, mode := range []string{"FIXTURE_OTHER_CHANGE", "FIXTURE_SOURCE_MISMATCH"} {
		t.Run(mode, func(t *testing.T) {
			h := fixtureHome(t)
			fakeCommand(t, "codex")
			t.Setenv(mode, "1")
			dir := filepath.Join(h, ".codex")
			put(t, filepath.Join(dir, "hooks.json"), `{"hooks":{"UserPromptSubmit":[{"hooks":[{"command":"unrelated"}]}]}}`)
			code, _, _ := runCaptured(t, h, "codex-task-title-apply")
			if code != 1 {
				t.Fatal("trust accepted")
			}
			if strings.Contains(get(t, filepath.Join(dir, "config.toml")), "trusted_hash") {
				t.Fatal("trust granted")
			}
		})
	}
}
func TestCodexProtocolErrorRedacted(t *testing.T) {
	h := fixtureHome(t)
	fakeCommand(t, "codex")
	t.Setenv("FIXTURE_CODEX_ERROR", "1")
	code, out, err := runCaptured(t, h, "codex-task-title-apply")
	if code != 1 || strings.Contains(out+err, "fixture-secret-protocol") {
		t.Fatal(code, out, err)
	}
}
func TestAbideHooksMergeAndTrust(t *testing.T) {
	h := fixtureHome(t)
	fakeCommand(t, "codex")
	dir := filepath.Join(h, ".codex")
	original := []byte(`{"hooks":{"PostToolUse":[{"matcher":"custom","extra":true,"hooks":[{"command":"old abide-hook.js"},{"command":"unrelated"}]}],"Other":[{"hooks":[{"command":"keep"}]}]}}`)
	b, e := mergeAbideHooks(original, h)
	if e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(dir, "hooks.json"), string(b))
	if e = abideTrust(h, dir); e != nil {
		t.Fatal(e)
	}
	entries, e := listHooks(h, filepath.Join(h, "bin/codex"), dir)
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, entry := range entries {
		if strings.Contains(entry.Command, "abide-hook.js") {
			count++
			if entry.TrustStatus != "trusted" || !entry.Enabled {
				t.Fatal(entry)
			}
		}
	}
	if count != 4 {
		t.Fatal(entries)
	}
	next, e := mergeAbideHooks(b, h)
	if e != nil || string(next) != string(b) {
		t.Fatal("not idempotent", e)
	}
}
func TestTrustRejectsInlineTable(t *testing.T) {
	s := `features = { hooks = false }`
	if _, e := enableHooks(s); e == nil {
		t.Fatal("accepted inline features")
	}
	s = "[hooks.state]\nfoo = { trusted_hash = \"old\", enabled = false }\n"
	if _, e := trustUpdate(s, []hookEntry{{Key: "foo", CurrentHash: "new"}}, true); e == nil {
		t.Fatal("accepted inline trust")
	}
}
