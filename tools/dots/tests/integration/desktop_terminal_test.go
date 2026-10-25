package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"dots/native/tests/testutil/testpty"
)

func (f *fixture) terminal(args ...string) *testpty.CLI {
	f.t.Helper()
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(f.env, "TERM=xterm-256color", "NO_COLOR=1", "FZF_DEFAULT_OPTS=--bind=start:execute(unsafe)")
	cmd.Dir = f.root
	return testpty.Start(f.t, cmd)
}
func TestNativeMenuCancellationAndPrivacy(t *testing.T) {
	for _, profile := range []string{"glass", "presentation", "unreadable"} {
		f := newFixture(t)
		path := filepath.Join(f.home, ".config/dots/style")
		if profile == "unreadable" {
			os.MkdirAll(path, 0700)
		} else {
			f.write(path, profile, 0600)
		}
		p := f.terminal("menu")
		p.Screen("Поиск")
		if strings.Contains(p.Text(), filepath.Base(f.root)) != (profile == "glass") {
			t.Fatalf("context privacy %s: %q", profile, p.Text())
		}
		if strings.Contains(p.Text(), "dots kube") {
			t.Fatal("desktop inherits Kubernetes branding")
		}
		p.Send("\x1b")
		p.Wait(0)
		if len(f.calls()) != 0 {
			t.Fatal("native menu executed external picker")
		}
	}
}
func TestNativeMenuKnownDispatchAndNestedCancel(t *testing.T) {
	for _, action := range []string{"git", "ports", "style", "config"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t)
			f.fake("lazygit", allRule("git fixture\n", 0))
			f.fake("lsof", allRule("ports fixture\n", 0))
			p := f.terminal("menu")
			p.Screen("Поиск")
			p.Send(action)
			p.Screen("Поиск › " + action)
			offset := len(p.Text())
			p.Send("\r")
			switch action {
			case "git", "ports":
				p.Wait(0)
			default:
				title := map[string]string{"style": "Стиль", "config": "Редактировать конфиг"}[action]
				p.After(offset, title)
				p.Send("\x1b")
				p.Wait(0)
			}
			calls := f.calls()
			if action == "git" || action == "ports" {
				if len(calls) != 1 {
					t.Fatalf("dispatch %+v", calls)
				}
			} else if len(calls) != 0 {
				t.Fatalf("nested cancel %+v", calls)
			}
		})
	}
}
func TestNativeMenuConfigUsesKnownPath(t *testing.T) {
	f := newFixture(t)
	f.fake("nvim", allRule("", 0))
	p := f.terminal("menu")
	p.Screen("Поиск")
	p.Send("config")
	p.Screen("Поиск › config")
	offset := len(p.Text())
	p.Send("\r")
	p.After(offset, "Редактировать конфиг")
	p.Send("herdr")
	p.Screen("Поиск › herdr")
	p.Send("\r")
	p.Wait(0)
	calls := f.calls()
	if len(calls) != 1 || calls[0].Name != "nvim" || !reflect.DeepEqual(calls[0].Args, []string{filepath.Join(f.root, "config/herdr/config.toml")}) {
		t.Fatalf("argv %+v", calls)
	}
}
func TestStandaloneClusterToolIsNotOfferedByDesktop(t *testing.T) {
	for _, args := range [][]string{{"kube"}, {"kube", "--help"}} {
		f := newFixture(t)
		out, diagnostic, code := f.run("", args...)
		if code != 2 || out != "" || diagnostic == "" || len(f.calls()) != 0 {
			t.Fatalf("retired desktop action %v: %d %q %q %+v", args, code, out, diagnostic, f.calls())
		}
	}
	for _, args := range [][]string{{"help"}, {"menu", "--preview", "--plain"}} {
		f := newFixture(t)
		out, diagnostic, code := f.run("", args...)
		if code != 0 || diagnostic != "" || strings.Contains(strings.ToLower(out), "kube") || len(f.calls()) != 0 {
			t.Fatalf("desktop advertises standalone tooling %v: %d %q %q", args, code, out, diagnostic)
		}
	}
}
func TestNativeClipboardConfirmationAndRace(t *testing.T) {
	for _, action := range []string{"cancel", "empty", "yes", "changed"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t)
			counts := []string{"1", "1", "1", "1"}
			if action == "changed" {
				counts[2] = "2"
			}
			f.fake("osascript", fakeRule{Args: []string{}, Prefix: true, Outputs: counts})
			f.fake("pbpaste", allRule(`{"a":1}`, 0))
			f.fake("pbcopy", allRule("", 0))
			p := f.terminal("clip", "json")
			p.Screen("Перезаписать clipboard?")
			want := 130
			switch action {
			case "yes":
				p.Send("yes\r")
				want = 0
			case "changed":
				p.Send("yes\r")
				want = 1
			case "empty":
				p.Send("\r")
			default:
				p.Send("\x1b")
			}
			p.Wait(want)
			copies := 0
			for _, call := range f.calls() {
				if call.Name == "pbcopy" {
					copies++
					if !strings.Contains(call.Input, `"a": 1`) {
						t.Fatal(call.Input)
					}
				}
			}
			if (copies == 1) != (action == "yes") {
				t.Fatalf("copies %d", copies)
			}
		})
	}
}
func TestNativeRefsUsesIndexesAndExplicitAction(t *testing.T) {
	f := newFixture(t)
	f.fake("tmux", fakeRule{Args: []string{"display-message"}, Prefix: true, Output: f.root + "\n"}, fakeRule{Args: []string{"capture-pane"}, Prefix: true, Output: "https://example.test/path\n"})
	f.fake("pbcopy", allRule("", 0))
	p := f.terminal("refs", "--pane", "%9")
	p.Screen("Ссылка из логов")
	offset := len(p.Text())
	p.Send("\r")
	p.After(offset, "Действие")
	p.Send("copy")
	p.Screen("Поиск › copy")
	p.Send("\r")
	p.Wait(0)
	calls := f.calls()
	if len(calls) != 3 || calls[2].Name != "pbcopy" || calls[2].Input != "https://example.test/path" {
		t.Fatalf("ref action %+v", calls)
	}
}

func TestNativeProjectsAndSessionsCanCancelWithoutOpening(t *testing.T) {
	f := newFixture(t)
	path := f.repo("fixture project")
	f.registry(path)
	f.fake("tmux", allRule("", 0))
	p := f.terminal("projects", "open")
	p.Screen("Проект / worktree")
	p.Send("\x1b")
	p.Wait(0)
	for _, call := range f.calls() {
		if call.Name != "tmux" {
			t.Fatalf("cancel opened project %+v", call)
		}
	}
	f = newFixture(t)
	f.fake("herdr", allRule(`{"result":{"snapshot":{"workspaces":[{"workspace_id":"ws-fixture","label":"Fixture","tab_count":1,"pane_count":1}],"panes":[]}}}`, 0))
	f.fake("sesh", allRule("[]", 0))
	p = f.terminal("sessions", "open")
	p.Screen("Session / workspace")
	p.Send("\x1b")
	p.Wait(0)
	if len(f.calls()) != 2 {
		t.Fatalf("cancel opened session %+v", f.calls())
	}
}

func TestNativeStatusCancelsInitialSubprocess(t *testing.T) {
	f := newFixture(t)
	f.registry()
	f.fake("tmux", fakeRule{Args: []string{}, Prefix: true, Block: true})
	p := f.terminal("status", "--interactive")
	p.Screen("Project status")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(f.bin, "blocked-tmux")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial subprocess did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.Send("\x1b")
	p.Wait(0)
	if len(f.calls()) != 1 {
		t.Fatalf("cancel kept collecting %+v", f.calls())
	}
}

func TestNativeSessionsControlNavigation(t *testing.T) {
	for _, tc := range []struct{ name, keys, workspace string }{
		{"next", "\x0e", "ws-second"},
		{"previous", "\x1b[B\x10", "ws-first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.fake("herdr",
				fakeRule{Args: []string{"api", "snapshot"}, Output: `{"result":{"snapshot":{"workspaces":[{"workspace_id":"ws-first","label":"Alpha","tab_count":1,"pane_count":1},{"workspace_id":"ws-second","label":"Beta","tab_count":1,"pane_count":1}],"panes":[]}}}`},
				fakeRule{Args: []string{"workspace", "focus", tc.workspace}, Output: `{"result":{}}`},
			)
			f.fake("sesh", allRule("[]", 0))
			p := f.terminal("sessions", "open")
			p.Screen("Session / workspace")
			p.Send(tc.keys + "\r")
			p.Wait(0)
			calls := f.calls()
			if len(calls) != 3 || calls[2].Name != "herdr" || !reflect.DeepEqual(calls[2].Args, []string{"workspace", "focus", tc.workspace}) {
				t.Fatalf("wrong workspace selected: %+v", calls)
			}
		})
	}
}

func TestNativeSessionsDefaultPickerIsLiveAndAllIsExplicit(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "all"}[all], func(t *testing.T) {
			f := newFixture(t)
			f.fake("herdr", allRule(`{"result":{"snapshot":{"workspaces":[{"workspace_id":"w1","label":"Live","tab_count":1,"pane_count":1}],"panes":[]}}}`, 0))
			rows := fmt.Sprintf(`[{"Src":"zoxide","Name":"History","Path":%q},{"Src":"config","Name":"Saved","Path":%q},{"Src":"tmux","Name":"Old","Path":%q}]`, f.home, f.home, f.home)
			f.fake("sesh", allRule(rows, 0))
			args := []string{"sessions", "open"}
			want := "1/1 rows"
			if all {
				args = append(args, "--all")
				want = "4/4 rows"
			}
			p := f.terminal(args...)
			p.Screen(want)
			if !all && strings.Contains(p.Text(), "History") {
				t.Fatal("directory history shown as a live session")
			}
			p.Send("\x1b")
			p.Wait(0)
		})
	}
}

func TestNativeStatusRefreshAndCancellation(t *testing.T) {
	f := newFixture(t)
	f.registry()
	f.fake("tmux", fakeRule{Args: []string{"list-panes"}, Prefix: true, Output: ""})
	f.fake("ps", allRule("", 0))
	f.fake("lsof", allRule("", 0))
	p := f.terminal("status", "--interactive")
	p.Screen("PROJECTS")
	offset := len(p.Text())
	p.Send("r")
	p.After(offset, "Сбор снимка")
	p.Send("\x1b")
	p.Wait(0)
	for _, call := range f.calls() {
		if call.Name == "gum" || call.Name == "fzf" {
			t.Fatal("external picker")
		}
	}
}
