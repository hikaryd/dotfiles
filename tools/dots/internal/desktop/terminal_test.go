package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"dots/native/internal/testpty"
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
	for _, action := range []string{"git", "ports", "style", "config", "kube"} {
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
				title := map[string]string{"style": "Стиль", "config": "Редактировать конфиг", "kube": "Окружение Kubernetes"}[action]
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
func TestNativeDesktopKubePinsReadOnlyArgv(t *testing.T) {
	f := newFixture(t)
	f.fake("dots-tools", allRule("", 0))
	p := f.terminal("kube")
	p.Screen("Окружение Kubernetes")
	p.Send("prod")
	p.Screen("Поиск › prod")
	p.Send("\x1b[F")
	offset := len(p.Text())
	p.Send("\r")
	p.After(offset, "Read-only действие")
	p.Send("pods")
	p.Screen("Поиск › pods")
	p.Send("\r")
	p.Wait(0)
	calls := f.calls()
	if len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []string{"kube", "kpprod", "get", "pods"}) {
		t.Fatalf("argv %+v", calls)
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
