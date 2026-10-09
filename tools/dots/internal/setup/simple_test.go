package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func runCaptured(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	out, e := os.CreateTemp(t.TempDir(), "out")
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	errfile, e := os.CreateTemp(t.TempDir(), "err")
	if e != nil {
		t.Fatal(e)
	}
	defer errfile.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, errfile
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	code := Run(root, args)
	return code, get(t, out.Name()), get(t, errfile.Name())
}
func fixtureHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	for _, k := range []string{"CODEX_CONFIG_DIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_DIR", "SRC", "DST", "CODEXBAR_CONFIG", "XDG_CONFIG_HOME", "HAVE_CLAUDE", "ANTHROPIC_BASE_URL", "Z_AI_API_KEY", "MAGIC_API_KEY", "KUBECONFIG_PATH", "TYPESAFE_AI_API_KEY", "OPENROUTER_API_KEY", "AI_GATEWAY_API_KEY"} {
		t.Setenv(k, "")
	}
	return h
}
func TestManagedLinks(t *testing.T) {
	for _, quoted := range []bool{false, true} {
		t.Run(map[bool]string{true: "quoted", false: "unquoted"}[quoted], func(t *testing.T) {
			root := t.TempDir()
			put(t, filepath.Join(root, "steps/terminal.yml"), "- link:\n    ~/.config/example:\n      path: config/example\n    ~/Library/Application Support/example/config.json:\n      path: config/example.json\n- shell:\n    - command: echo not-a-link\n")
			want := "~/.config/example\n~/Library/Application Support/example/config.json\n"
			if quoted {
				put(t, filepath.Join(root, "steps/quoted.yml"), "- link:\n    \"~/quoted config\":\n      path: config/example\n")
				want = "~/quoted config\n" + want
			}
			status, out, err := runCaptured(t, "ignored", "managed-links", root)
			if status != 0 || out != want || err != "" {
				t.Fatalf("%d %q %q", status, out, err)
			}
		})
	}
}
func TestJSONKeysSafeAndOrdered(t *testing.T) {
	p := filepath.Join(t.TempDir(), "servers.json")
	put(t, p, `{"z":{"secret":"fixture-token"},"a":{},"quoted key":{}}`)
	code, out, err := runCaptured(t, "", "json-keys", p)
	if code != 0 || out != "z\na\nquoted key\n" || err != "" {
		t.Fatal(code, out, err)
	}
	for _, b := range []string{"", `["fixture-token"]`, `{"secret":"fixture-token`} {
		put(t, p, b)
		code, out, err = runCaptured(t, "", "json-keys", p)
		if code != 1 || out != "" || strings.Contains(err, "fixture-token") {
			t.Fatal(code, out, err)
		}
	}
}
func TestCodexbarResolution(t *testing.T) {
	for _, variant := range []string{"default", "legacy", "current", "xdg", "relative-xdg", "override"} {
		t.Run(variant, func(t *testing.T) {
			h := fixtureHome(t)
			current := filepath.Join(h, ".config/codexbar/config.json")
			legacy := filepath.Join(h, ".codexbar/config.json")
			want := current
			if variant != "default" {
				put(t, legacy, `{"version":1,"providers":[{"id":"other","enabled":false}],"account":"fixture-account"}`)
				want = legacy
			}
			switch variant {
			case "current":
				put(t, current, `{"providers":[{"id":"codex","source":"web","extra":true}]}`)
				want = current
			case "xdg":
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, "xdg"))
				want = filepath.Join(h, "xdg/codexbar/config.json")
			case "relative-xdg":
				t.Setenv("XDG_CONFIG_HOME", "relative")
			case "override":
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, "xdg"))
				t.Setenv("CODEXBAR_CONFIG", "  ~/custom.json  ")
				want = filepath.Join(h, "custom.json")
			}
			code, out, err := runCaptured(t, "", "codexbar-config")
			if code != 0 || strings.TrimSpace(out) != want {
				t.Fatal(code, out, err)
			}
			d, e := readObject(want, false)
			if e != nil {
				t.Fatal(e)
			}
			providers := d["providers"].([]any)
			found := false
			for _, v := range providers {
				m := v.(map[string]any)
				if m["id"] == "codex" {
					found = m["enabled"] == true && m["source"] == "cli"
				}
			}
			if !found {
				t.Fatal(d)
			}
			if variant == "legacy" && d["account"] != "fixture-account" {
				t.Fatal(d)
			}
		})
	}
}
func TestCodexbarMalformedAndSymlink(t *testing.T) {
	h := fixtureHome(t)
	p := filepath.Join(h, "config.json")
	t.Setenv("CODEXBAR_CONFIG", p)
	put(t, p, "")
	code, _, _ := runCaptured(t, "", "codexbar-config")
	if code != 1 || get(t, p) != "" {
		t.Fatal("empty input mutated")
	}
	target := filepath.Join(h, "target.json")
	put(t, target, `{"providers":[]}`)
	os.Remove(p)
	if e := os.Symlink(target, p); e != nil {
		t.Fatal(e)
	}
	code, _, _ = runCaptured(t, "", "codexbar-config")
	if code != 0 || get(t, target) != `{"providers":[]}` {
		t.Fatal("target changed")
	}
	i, _ := os.Lstat(p)
	if i.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected atomic symlink replacement")
	}
}
func TestLookHotkey(t *testing.T) {
	h := fixtureHome(t)
	p := filepath.Join(h, ".look/config")
	put(t, p, "other=keep\r\n launcher_hotkey =old\nlauncher_hotkey=x\nlast=value")
	if Run("", []string{"look-hotkey"}) != 0 {
		t.Fatal("apply")
	}
	want := "other=keep\r\nlauncher_hotkey=alt+a\nlast=value"
	if get(t, p) != want {
		t.Fatal(get(t, p))
	}
	if Run("", []string{"look-hotkey"}) != 0 || get(t, p) != want {
		t.Fatal("idempotence")
	}
	put(t, p, "last=value")
	Run("", []string{"look-hotkey"})
	if get(t, p) != "last=value\nlauncher_hotkey=alt+a\n" {
		t.Fatal(get(t, p))
	}
}
func TestInstructionsPreserveAndBackup(t *testing.T) {
	h := fixtureHome(t)
	root := filepath.Join(h, "root")
	p := filepath.Join(h, "codex/AGENTS.md")
	t.Setenv("CODEX_CONFIG_DIR", filepath.Dir(p))
	put(t, filepath.Join(root, "config/codex/search-instructions.md"), "new rules\n")
	put(t, p, "mine\n<!-- BEGIN DOTS SEARCH -->\nold\n<!-- END DOTS SEARCH -->\nafter\n")
	if Run(root, []string{"codex-instructions-apply"}) != 0 {
		t.Fatal("apply")
	}
	if get(t, p) != "mine\n<!-- BEGIN DOTS SEARCH -->\nnew rules\n<!-- END DOTS SEARCH -->\nafter\n" {
		t.Fatal(get(t, p))
	}
	before := tree(t, filepath.Dir(p))
	if Run(root, []string{"codex-instructions-apply"}) != 0 || !reflect.DeepEqual(before, tree(t, filepath.Dir(p))) {
		t.Fatal("not idempotent")
	}
	put(t, p, "<!-- BEGIN DOTS SEARCH -->\nno end")
	if Run(root, []string{"codex-instructions-apply"}) != 1 {
		t.Fatal("incomplete block")
	}
}
func TestWaterfoxProfilesAndBlocks(t *testing.T) {
	h := fixtureHome(t)
	root := filepath.Join(h, "root")
	base := filepath.Join(h, "Library/Application Support/Waterfox")
	put(t, filepath.Join(root, "config/waterfox/user.js"), "user_pref(\"ours\", true);\n")
	put(t, filepath.Join(base, "profiles.ini"), "[Profile0]\nPath=Profiles/used\nIsRelative=1\n[Profile1]\nPath=Profiles/unused\n[Profile2]\nPath=Profiles/used\n[Install123]\nDefault=unused\n")
	put(t, filepath.Join(base, "Profiles/used/prefs.js"), "used")
	p := filepath.Join(base, "Profiles/used/user.js")
	put(t, p, "unrelated\r\n// BEGIN dots Waterfox\r\nold\r\n// END dots Waterfox\r\ntrailing")
	if code, _, err := runCaptured(t, root, "waterfox-apply"); code != 0 {
		t.Fatal(err)
	}
	if get(t, p) != "unrelated\r\n// BEGIN dots Waterfox\nuser_pref(\"ours\", true);\n// END dots Waterfox\ntrailing" {
		t.Fatal(get(t, p))
	}
	before := tree(t, base)
	if Run(root, []string{"waterfox-apply"}) != 0 || !reflect.DeepEqual(before, tree(t, base)) {
		t.Fatal("idempotence")
	}
	put(t, p, "// BEGIN dots Waterfox\nmissing end")
	if Run(root, []string{"waterfox-apply"}) != 1 || get(t, p) != "// BEGIN dots Waterfox\nmissing end" {
		t.Fatal("malformed mutation")
	}
}

func TestWaterfoxRejectsSymlinkAndInvalidRegistry(t *testing.T) {
	for _, mode := range []string{"relative-absolute", "absolute-relative", "bad-bool", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			h := fixtureHome(t)
			root := filepath.Join(h, "root")
			base := filepath.Join(h, "Library/Application Support/Waterfox")
			put(t, filepath.Join(root, "config/waterfox/user.js"), "managed")
			profile := filepath.Join(base, "profile")
			put(t, filepath.Join(profile, "prefs.js"), "used")
			ini := "[Profile0]\nPath=profile\nIsRelative=1\n"
			switch mode {
			case "relative-absolute":
				ini = "[Profile0]\nPath=" + profile + "\nIsRelative=1\n"
			case "absolute-relative":
				ini = "[Profile0]\nPath=profile\nIsRelative=0\n"
			case "bad-bool":
				ini = "[Profile0]\nPath=profile\nIsRelative=maybe\n"
			case "symlink":
				target := filepath.Join(h, "external.js")
				put(t, target, "keep")
				if e := os.Symlink(target, filepath.Join(profile, "user.js")); e != nil {
					t.Fatal(e)
				}
			}
			put(t, filepath.Join(base, "profiles.ini"), ini)
			before := tree(t, base)
			code, _, _ := runCaptured(t, root, "waterfox-apply")
			if code != 1 || !reflect.DeepEqual(before, tree(t, base)) {
				t.Fatal("invalid Waterfox state mutated")
			}
		})
	}
}
func TestWaterfoxIndentedRegistryAndAbsentProfiles(t *testing.T) {
	h := fixtureHome(t)
	root := filepath.Join(h, "root")
	base := filepath.Join(h, "Library/Application Support/Waterfox")
	put(t, filepath.Join(root, "config/waterfox/user.js"), "managed")
	code, out, err := runCaptured(t, root, "waterfox-apply")
	if code != 0 || out != "Использованных профилей Waterfox нет; пропуск.\n" {
		t.Fatal(code, out, err)
	}
	put(t, filepath.Join(base, "profiles.ini"), "[DEFAULT]\nIsRelative=1\n[Profile0]\n  Path=used\n  IsRelative=true\n")
	put(t, filepath.Join(base, "used/prefs.js"), "used")
	code, _, err = runCaptured(t, root, "waterfox-apply")
	if code != 0 {
		t.Fatal(err)
	}
	i, e := os.Stat(filepath.Join(base, "used/user.js"))
	if e != nil || i.Mode().Perm() != 0600 {
		t.Fatal(e)
	}
}

func TestManagedLinksAliasesAndMalformed(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "steps/aliases.yml")
	put(t, p, "- &links\n  link:\n    '~/quoted: name': config\n- *links\n")
	code, out, err := runCaptured(t, root, "managed-links")
	if code != 0 || out != "~/quoted: name\n~/quoted: name\n" {
		t.Fatal(code, out, err)
	}
	put(t, p, "- link:\n    ~/first: value\n---\n- link:\n    ~/second: value\n")
	code, out, err = runCaptured(t, root, "managed-links")
	if code != 1 || out != "" {
		t.Fatal(code, out, err)
	}
}
