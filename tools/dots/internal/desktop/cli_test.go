package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLIClipStdinAndFailures(t *testing.T) {
	f := newFixture(t)
	out, err, code := f.run(`{"z":1,"a":"hi"}`, "clip", "compact", "--stdin", "--stdout")
	if code != 0 || out != "{\"z\":1,\"a\":\"hi\"}\n" || err != "" {
		t.Fatalf("%d %q %q", code, out, err)
	}
	for _, args := range [][]string{{"unknown"}, {"clip", "json", "--write", "--stdout"}, {"status", "--json", "--interactive"}, {"status", "--interactive"}, {"projects", "list", "extra"}, {"notify", "--status", "256"}, {"refs", "--lines", "no"}} {
		_, _, code := f.run("", args...)
		if code != 2 {
			t.Errorf("%v returned %d", args, code)
		}
	}
	out, err, code = f.run(`{"token":"NEVER_ECHO","token":2}`, "clip", "json", "--stdin", "--stdout")
	if code != 1 || out != "" || strings.Contains(err, "NEVER_ECHO") {
		t.Fatalf("unsafe invalid JSON: %d %q %q", code, out, err)
	}
	_, _, code = f.run("", "clip", "json", "--stdout")
	if code != 1 {
		t.Fatal("missing pbpaste must fail")
	}
	if len(f.calls()) != 0 {
		t.Fatal("stdin path invoked a clipboard command")
	}
}
func TestClipboardWriteAndConcurrentChange(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "write", true: "changed"}[changed], func(t *testing.T) {
			f := newFixture(t)
			counts := []string{"1\n", "1\n", "1\n", "1\n"}
			if changed {
				counts[2] = "2\n"
			}
			f.fake("osascript", fakeRule{Args: []string{}, Prefix: true, Outputs: counts})
			f.fake("pbpaste", allRule(`{"secret":"fixture-value"}`, 0))
			f.fake("pbcopy", allRule("", 0))
			out, err, code := f.run("", "clip", "redact", "--write")
			if out != "" || strings.Contains(err, "fixture-value") {
				t.Fatalf("leaked clipboard: %q %q", out, err)
			}
			copies := []fakeCall{}
			for _, c := range f.calls() {
				if c.Name == "pbcopy" {
					copies = append(copies, c)
				}
			}
			if changed {
				if code != 1 || len(copies) != 0 {
					t.Fatalf("changed pasteboard was written: %d %+v", code, copies)
				}
			} else {
				if code != 0 || len(copies) != 1 || !strings.Contains(copies[0].Input, `"[REDACTED]"`) {
					t.Fatalf("write failed: %d %q %+v", code, err, copies)
				}
			}
		})
	}
}
func TestClipboardABACountAndSameCountContent(t *testing.T) {
	for _, counts := range [][]string{{"10", "11"}, {"10", "10", "10", "10"}} {
		f := newFixture(t)
		f.fake("osascript", fakeRule{Args: []string{}, Prefix: true, Outputs: counts})
		f.fake("pbpaste", fakeRule{Args: []string{}, Prefix: true, Outputs: []string{`{"a":1}`, `{"a":2}`}})
		f.fake("pbcopy", allRule("", 0))
		_, _, code := f.run("", "clip", "json", "--write")
		if code != 1 {
			t.Fatal("race accepted")
		}
		for _, c := range f.calls() {
			if c.Name == "pbcopy" {
				t.Fatal("raced copy")
			}
		}
	}
}
func TestPickerWhitelistAndCancellation(t *testing.T) {
	for _, out := range []string{"evil · run\n", "projects · Рабочие пространства; rm\n", ""} {
		f := newFixture(t)
		f.fake("gum", allRule(out, 0))
		_, _, code := f.run("", "menu")
		if code != 0 || len(f.calls()) != 1 {
			t.Fatalf("menu did not cancel: %d %+v", code, f.calls())
		}
	}
	f := newFixture(t)
	f.fake("gum", allRule("not-a-mode", 0))
	_, _, code := f.run("", "clip")
	if code != 130 {
		t.Fatalf("clip cancellation %d", code)
	}
}
func TestPortsExitAndArgv(t *testing.T) {
	for _, status := range []int{0, 1, 3} {
		f := newFixture(t)
		f.fake("lsof", allRule("listeners\n", status))
		out, _, code := f.run("", "ports")
		want := status
		if want == 1 {
			want = 0
		}
		if code != want || out != "listeners\n" {
			t.Fatalf("ports: %d %q", code, out)
		}
		if !reflect.DeepEqual(f.calls()[0].Args, []string{"-nP", "-iTCP", "-sTCP:LISTEN"}) {
			t.Fatal("ports argv changed")
		}
	}
	f := newFixture(t)
	_, _, code := f.run("", "ports")
	if code != 127 {
		t.Fatal("missing lsof must return 127")
	}
}
func TestPrivateInitCheckSafety(t *testing.T) {
	f := newFixture(t)
	for _, p := range []string{"config/zsh/private.example.zsh", "config/claude/mcp-secrets.example.sh", "config/codex/mcp-secrets.example.sh"} {
		f.write(filepath.Join(f.root, p), "export KEY=''\n", 0600)
	}
	out, err, code := f.run("", "private", "init")
	if code != 0 {
		t.Fatalf("init %s %s", out, err)
	}
	for _, p := range []string{".config/dots/private.zsh", ".config/git/private.inc", ".claude/.mcp-secrets", ".codex/.mcp-secrets"} {
		path := filepath.Join(f.home, p)
		info, e := os.Stat(path)
		if e != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("mode %s: %v", p, e)
		}
		f.write(path, "export KEY='fixture-private-value'\n", 0600)
	}
	out, err, code = f.run("", "private", "check")
	if code != 0 || strings.Contains(out+err, "fixture-private-value") {
		t.Fatalf("check %d %q %q", code, out, err)
	}
	_, _, code = f.run("", "private", "init")
	if code != 0 {
		t.Fatal("idempotent init failed")
	}
	b, _ := os.ReadFile(filepath.Join(f.home, ".config/dots/private.zsh"))
	if !strings.Contains(string(b), "fixture-private-value") {
		t.Fatal("overwrote private settings")
	}
	os.Chmod(filepath.Join(f.home, ".config/dots/private.zsh"), 0644)
	out, _, code = f.run("", "private", "check")
	if code != 1 || !strings.Contains(out, "600") {
		t.Fatal("unsafe mode accepted")
	}
}
func TestPrivateRejectsAllPathsBeforeInit(t *testing.T) {
	for _, kind := range []string{"relative", "repository", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			target := "relative"
			switch kind {
			case "repository":
				target = filepath.Join(f.root, "private")
			case "symlink":
				target = filepath.Join(f.home, "link")
				os.Symlink(filepath.Join(f.home, "outside"), target)
			case "directory":
				target = f.home
			}
			f.env = append(f.env, "DOTS_PRIVATE_ZSH="+target)
			_, _, code := f.run("", "private", "init")
			if code != 1 {
				t.Fatal("unsafe path accepted")
			}
			if exists(filepath.Join(f.home, ".config/git/private.inc")) {
				t.Fatal("partial init before validation")
			}
		})
	}
}
func TestPrivateSymlinkParentAndUnfilled(t *testing.T) {
	f := newFixture(t)
	f.localEnv()
	link := filepath.Join(f.home, "linked")
	os.Symlink(f.root, link)
	if validatePrivatePath(f.root, filepath.Join(link, "private")) == nil {
		t.Fatal("repository via symlink accepted")
	}
	for _, content := range []string{"# comment\n", "KEY=''\n", "KEY=\"\"\n", "KEY='your-token'\n", "KEY='dev-context'\n"} {
		p := filepath.Join(f.home, "private")
		f.write(p, content, 0600)
		if checkPrivate(p) == nil {
			t.Fatalf("accepted %q", content)
		}
	}
}
func TestNotifyMetadataOnly(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "TMUX=fixture", "TMUX_PANE=%1")
	f.fake("tmux", allRule("", 0))
	_, _, code := f.run("", "notify", "--seconds", "29")
	if code != 0 || len(f.calls()) != 0 {
		t.Fatal("short command should do nothing")
	}
	out, err, code := f.run("", "notify", "--test", "--seconds", "31", "--status", "2")
	if code != 1 || !strings.Contains(out, "no-ghostty-client") {
		t.Fatalf("%d %q %q", code, out, err)
	}
	b, e := os.ReadFile(filepath.Join(f.home, ".local/state/dots/last-notification.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	if len(m) != 4 || m["result"] != "no-ghostty-client" || m["exit_code"] != float64(2) {
		t.Fatalf("metadata %+v", m)
	}
	if strings.Contains(string(notification(0, 30, "a;\x1b\nB")), "a;\x1b") {
		t.Fatal("notification control injection")
	}
}

func TestClipboardConfirmationWithoutTerminalCancels(t *testing.T) {
	f := newFixture(t)
	f.fake("osascript", allRule("1", 0))
	f.fake("pbpaste", allRule(`{"fixture":"hello\nworld"}`, 0))
	f.fake("pbcopy", allRule("", 0))
	out, err, code := f.run("", "clip", "json")
	if code != 130 || out != "" || !strings.Contains(err, `\n`) {
		t.Fatalf("confirmation %d %q %q", code, out, err)
	}
	for _, c := range f.calls() {
		if c.Name == "pbcopy" {
			t.Fatal("clipboard written without confirmation")
		}
	}
}
func TestKubeWhitelistArgv(t *testing.T) {
	f := newFixture(t)
	f.fake("gum", fakeRule{Args: []string{}, Prefix: true, Outputs: []string{"prod · PROD · только чтение\n", "pods · Список pod\n"}})
	f.fake("dots-tools", allRule("", 0))
	_, err, code := f.run("", "kube")
	if code != 0 {
		t.Fatal(err)
	}
	calls := f.calls()
	last := calls[len(calls)-1]
	if last.Name != "dots-tools" || !reflect.DeepEqual(last.Args, []string{"kube", "kpprod", "get", "pods"}) {
		t.Fatalf("kube argv %+v", last)
	}
}
func TestMenuConfigUsesKnownPath(t *testing.T) {
	f := newFixture(t)
	f.fake("gum", fakeRule{Args: []string{}, Prefix: true, Outputs: []string{"config · Настройка / Открыть конфиг\n", "herdr · config/herdr/config.toml\n"}})
	f.fake("nvim", allRule("", 0))
	_, err, code := f.run("", "menu")
	if code != 0 {
		t.Fatal(err)
	}
	calls := f.calls()
	last := calls[len(calls)-1]
	if last.Name != "nvim" || !reflect.DeepEqual(last.Args, []string{filepath.Join(f.root, "config/herdr/config.toml")}) {
		t.Fatalf("menu argv %+v", last)
	}
}
