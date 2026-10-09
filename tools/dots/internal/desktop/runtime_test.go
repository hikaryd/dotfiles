package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTmuxStyleRestoresOnlyOwnedOverrides(t *testing.T) {
	f := newFixture(t)
	saved := map[string]string{"status-left": "old-left", "window-status-format": "old-window", "window-status-current-format": "old-current"}
	b, _ := json.Marshal(saved)
	rules := []fakeRule{{Args: []string{"list-sessions"}}, {Args: []string{"show-options", "-gqv", snapshotOption}, Outputs: []string{"", string(b)}}}
	for i, p := range presentation {
		second := p[1]
		if i == 1 {
			second = "user changed this"
		}
		rules = append(rules, fakeRule{Args: []string{"show-options", "-gqv", p[0]}, Outputs: []string{saved[p[0]] + "\n", second + "\n"}})
	}
	rules = append(rules, fakeRule{Args: []string{"set-option"}, Prefix: true})
	f.fake("tmux", rules...)
	f.fake("pgrep", allRule("", 1))
	out, err, code := f.run("", "style", "presentation")
	if code != 0 || !strings.Contains(out, "presentation: saved.") {
		t.Fatalf("%d %s %s", code, out, err)
	}
	b, _ = os.ReadFile(filepath.Join(f.home, ".config/dots/ghostty-style.conf"))
	if !strings.Contains(string(b), "font-size = 20") || !strings.Contains(string(b), "background-opacity = 1") {
		t.Fatal(string(b))
	}
	_, err, code = f.run("", "style", "focus")
	if code != 0 {
		t.Fatal(err)
	}
	restored := map[string]string{}
	for _, c := range f.calls() {
		if c.Name == "tmux" && len(c.Args) == 4 && c.Args[0] == "set-option" && strings.HasPrefix(c.Args[3], "old-") {
			restored[c.Args[2]] = c.Args[3]
		}
	}
	want := map[string]string{"status-left": "old-left", "window-status-current-format": "old-current"}
	if !reflect.DeepEqual(restored, want) {
		t.Fatalf("restore %+v", restored)
	}
	b, _ = os.ReadFile(filepath.Join(f.home, ".config/dots/ghostty-style.conf"))
	if strings.Contains(string(b), "font-size") {
		t.Fatal("presentation override remains")
	}
}
func TestHerdrStatusPresentationAndTruncation(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.home, ".config/dots/style"), "presentation\n", 0600)
	f.env = append(f.env, "HERDR_ACTIVE_WORKSPACE_ID=w2")
	out, _, code := f.run("", "herdr-status")
	if code != 0 || out != "PRESENTATION\n" || len(f.calls()) != 0 {
		t.Fatalf("presentation %d %q", code, out)
	}
	f.write(filepath.Join(f.home, ".config/dots/style"), "glass\n", 0600)
	f.fake("herdr", fakeRule{Args: []string{"workspace", "get", "w2"}, Output: `{"result":{"workspace":{"label":"hello\u001b\n世界123456789012345678901234567890"}}}`})
	out, err, code := f.run("", "herdr-status")
	if code != 0 || strings.ContainsAny(out, "\x1b") || out != " hello世界123456789012345\n" {
		t.Fatalf("label %d %q %q", code, out, err)
	}
}
func TestNuCacheOwnershipAndLaunch(t *testing.T) {
	f := newFixture(t)
	local := filepath.Join(f.home, ".local/bin")
	os.MkdirAll(local, 0700)
	f.fake("nu", allRule("NU_STARTED\n", 0))
	f.fake("atuin", fakeRule{Args: []string{"init", "nu", "--disable-up-arrow", "--disable-ai"}, Output: "# fixture atuin\n"})
	f.fake("dots-zsh", fakeRule{Args: []string{"--list"}, Output: "hello\nvalid.name\nvs\ndots-nu\n_bad\n;bad\nhello\n"})
	for _, name := range []string{"nu", "atuin"} {
		os.Symlink(filepath.Join(f.bin, name), filepath.Join(local, name))
	}
	os.MkdirAll(filepath.Join(f.root, "scripts"), 0700)
	os.Symlink(filepath.Join(f.bin, "dots-zsh"), filepath.Join(f.root, "scripts/dots-zsh"))
	cache := filepath.Join(f.home, ".cache/dots/zsh-bridge/bin")
	os.MkdirAll(cache, 0700)
	f.write(filepath.Join(cache, "hello"), "foreign", 0600)
	os.Symlink(filepath.Join(f.root, "scripts/dots-zsh"), filepath.Join(cache, "obsolete"))
	os.Symlink("/fixture/foreign-target", filepath.Join(cache, "foreign"))
	out, err, code := f.run("", "nu", "-c", "echo fixture")
	if code != 0 || out != "NU_STARTED\n" {
		t.Fatalf("nu %d %s %s", code, out, err)
	}
	if exists(filepath.Join(cache, "obsolete")) || exists(filepath.Join(cache, "vs")) || exists(filepath.Join(cache, "_bad")) {
		t.Fatal("invalid or stale bridge entries retained")
	}
	if !exists(filepath.Join(cache, "foreign")) {
		t.Fatal("foreign link removed")
	}
	b, _ := os.ReadFile(filepath.Join(cache, "hello"))
	if string(b) != "foreign" {
		t.Fatal("foreign file replaced")
	}
	if p, e := os.Readlink(filepath.Join(cache, "valid.name")); e != nil || p != filepath.Join(f.root, "scripts/dots-zsh") {
		t.Fatal("valid link missing")
	}
	calls := f.calls()
	last := calls[len(calls)-1]
	if last.Name != "nu" || !reflect.DeepEqual(last.Args, []string{"--config", filepath.Join(f.root, "config/nushell/config.nu"), "-c", "echo fixture"}) || last.Env["DOTS_ROOT"] != f.root || last.Env["DOTS_ZSH_BIN"] != cache || !strings.HasPrefix(last.Env["PATH"], cache+string(os.PathListSeparator)) {
		t.Fatalf("nu launch %+v", last)
	}
	_, err, code = f.run("", "nu", "-c", "echo second")
	if code != 0 {
		t.Fatal(err)
	}
	listCalls := 0
	atuinCalls := 0
	for _, c := range f.calls() {
		if c.Name == "dots-zsh" {
			listCalls++
		}
		if c.Name == "atuin" {
			atuinCalls++
		}
	}
	if listCalls != 1 || atuinCalls != 1 {
		t.Fatalf("cache not reused: bridge=%d atuin=%d", listCalls, atuinCalls)
	}
}
func TestNuRejectsSymlinkCache(t *testing.T) {
	f := newFixture(t)
	f.fake("nu", allRule("SHOULD_NOT_START", 0))
	os.MkdirAll(filepath.Join(f.home, ".local/bin"), 0700)
	os.Symlink(filepath.Join(f.bin, "nu"), filepath.Join(f.home, ".local/bin/nu"))
	os.MkdirAll(filepath.Join(f.home, ".cache/dots"), 0700)
	os.Symlink(f.root, filepath.Join(f.home, ".cache/dots/zsh-bridge"))
	out, _, code := f.run("", "nu")
	if code != 1 || out != "" || len(f.calls()) != 0 {
		t.Fatalf("unsafe cache accepted %d %s", code, out)
	}
}
func TestDoctorReadOnlyFixtureAndStrict(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.root, "steps/terminal.yml"), "- link:\n    ~/.zshrc:\n      path: config/zsh/.zshrc\n", 0600)
	f.write(filepath.Join(f.root, "config/zsh/.zshrc"), "# fixture\n", 0600)
	os.Symlink(filepath.Join(f.root, "config/zsh/.zshrc"), filepath.Join(f.home, ".zshrc"))
	for _, p := range []string{"config/sesh/sesh.toml", "config/herdr/config.toml"} {
		f.write(filepath.Join(f.root, p), "enabled = true\n", 0600)
	}
	f.write(filepath.Join(f.home, ".config/atuin/config.toml"), "auto_sync = false\nupdate_check = false\n", 0600)
	f.write(filepath.Join(f.home, ".config/dots/style"), "glass\n", 0600)
	f.write(filepath.Join(f.home, ".config/dots/ghostty-style.conf"), "# fixture", 0600)
	f.registry()
	f.write(filepath.Join(f.home, ".ssh/id_fixture"), "never-read-fixture-secret", 0600)
	for _, name := range []string{"zsh", "git", "dots-tools", "tmux", "herdr", "sesh", "nvim", "fzf", "dots", "gum", "zoxide", "eza", "bat", "rg", "fd", "yazi", "lazygit", "atuin"} {
		f.fake(name, allRule("", 0))
	}
	out, err, code := f.run("", "doctor", "--json", "--strict")
	var report struct {
		Checks  []doctorCheck
		Summary map[string]int
	}
	if code != 0 || json.Unmarshal([]byte(out), &report) != nil || report.Summary["FAIL"] != 0 || report.Summary["WARN"] != 0 {
		t.Fatalf("doctor %d %s %s", code, out, err)
	}
	if strings.Contains(out, "never-read-fixture-secret") {
		t.Fatal("key bytes exposed")
	}
	calls := f.calls()
	if len(calls) != 1 || calls[0].Name != "zsh" || !reflect.DeepEqual(calls[0].Args, []string{"-f", "-n", filepath.Join(f.root, "config/zsh/.zshrc")}) {
		t.Fatalf("doctor executed unsafe commands %+v", calls)
	}
	os.Remove(filepath.Join(f.bin, "gum"))
	_, _, code = f.run("", "doctor", "--strict")
	if code != 1 {
		t.Fatal("strict ignored missing optional command")
	}
	_, _, code = f.run("", "doctor")
	if code != 0 {
		t.Fatal("non-strict warning failed")
	}
}

func TestStyleUnreadableStateFails(t *testing.T) {
	f := newFixture(t)
	if e := os.MkdirAll(filepath.Join(f.home, ".config/dots/style"), 0700); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"style", "status"}, {"herdr-status"}, {"tmux-style", "--current"}} {
		_, _, code := f.run("", args...)
		if code != 1 {
			t.Errorf("unreadable style accepted %v: %d", args, code)
		}
	}
}
