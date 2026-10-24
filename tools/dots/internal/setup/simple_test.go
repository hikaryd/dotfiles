package setup

import (
	"encoding/json"
	"fmt"
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
	for _, k := range []string{"XDG_CONFIG_HOME", "PI_CODING_AGENT_DIR"} {
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
func fakeDefaults() int {
	if os.Getenv("FIXTURE_DEFAULTS_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "fixture-secret-defaults-error")
		return 17
	}
	b, e := json.Marshal(os.Args[1:])
	if e != nil || os.WriteFile(filepath.Join(home(), "defaults-argv.json"), b, 0600) != nil {
		return 99
	}
	return 0
}

func TestTinycastHotkey(t *testing.T) {
	h := fixtureHome(t)
	fakeCommand(t, "defaults")
	want := []string{"write", "com.tinycast.app", "hotkey.togglePalette", "-string",
		`{"combo":{"_0":{"carbonModifiers":2048,"carbonKeyCode":0}}}`}
	for range 2 {
		code, out, diagnostic := runCaptured(t, "", "tinycast-hotkey")
		if code != 0 || diagnostic != "" || !strings.Contains(out, "Option+A") {
			t.Fatal(code, out, diagnostic)
		}
		var args []string
		if e := json.Unmarshal([]byte(get(t, filepath.Join(h, "defaults-argv.json"))), &args); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("defaults arguments: %q", args)
		}
	}
	if _, e := os.Stat(filepath.Join(h, ".look")); !os.IsNotExist(e) {
		t.Fatal("created obsolete Look configuration")
	}
}

func TestTinycastHotkeyDefaultsFailure(t *testing.T) {
	for _, mode := range []string{"missing", "failed"} {
		t.Run(mode, func(t *testing.T) {
			fixtureHome(t)
			if mode == "failed" {
				fakeCommand(t, "defaults")
				t.Setenv("FIXTURE_DEFAULTS_FAIL", "1")
			} else {
				t.Setenv("PATH", t.TempDir())
			}
			code, out, diagnostic := runCaptured(t, "", "tinycast-hotkey")
			if code != 1 || out != "" || !strings.Contains(diagnostic, "external command failed") || strings.Contains(diagnostic, "fixture-secret") {
				t.Fatal(code, out, diagnostic)
			}
		})
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
