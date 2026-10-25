package desktop

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRmuxSessionsListDoesNotRequireHerdr(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "RMUX=/tmp/rmux work.sock,42,0", "RMUX_PANE=%7", "HERDR_ENV=1", "HERDR_SOCKET_PATH=/stale/herdr")
	f.fake("rmux", fakeRule{Args: []string{"-S", "/tmp/rmux work.sock", "list-sessions", "--json"}, Output: `[{"session_id":"$2","session_name":"project with spaces","session_windows":3}]`},
		fakeRule{Args: []string{"-S", "/tmp/rmux work.sock", "list-panes", "-a", "-F", "#{session_id}\t#{pane_current_path}"}, Output: "$2\t" + f.root + "\n"})
	f.fake("sesh", fakeRule{Args: []string{"list", "-j"}, Output: "[]"})
	out, err, code := f.run("", "sessions", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, err)
	}
	var rows []session
	if json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 1 || rows[0].Source != "rmux" || rows[0].Fresh || rows[0].ID != "rmux:$2" || ptrText(rows[0].Path) != f.root {
		t.Fatalf("inventory=%s", out)
	}
}

func TestRmuxSessionOpenTargetsPopupClientAndDoesNotRecreate(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "RMUX=/tmp/rmux.sock,42,0", "DOTS_MUX_CLIENT=/dev/ttys017")
	f.fake("rmux", fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "list-sessions", "--json"}, Output: `[{"session_id":"$2","session_name":"existing","session_windows":1}]`},
		fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "list-panes", "-a", "-F", "#{session_id}\t#{pane_current_path}"}, Output: "$2\t" + f.root + "\n"},
		fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "switch-client", "-c", "/dev/ttys017", "-t", "$2"}})
	f.fake("sesh", fakeRule{Args: []string{"list", "-j"}, Output: "[]"})
	_, err, code := f.run("", "sessions", "open", "existing")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, err)
	}
	for _, c := range f.calls() {
		if strings.Contains(strings.Join(c.Args, " "), "new-session") {
			t.Fatal("recreated live session")
		}
	}
}

func TestRmuxRefsCaptureIgnoresInheritedHerdrAndTmux(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "RMUX=/tmp/rmux.sock,42,0", "RMUX_PANE=%7", "TMUX_PANE=%99", "HERDR_ACTIVE_PANE_ID=w1:p1")
	f.fake("rmux", fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "display-message", "-p", "-t", "%7", "#{pane_current_path}"}, Output: f.root + "\n"},
		fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "capture-pane", "-p", "-J", "-S", "-200", "-t", "%7"}, Output: "https://example.com/rmux\n"})
	out, err, code := f.run("", "refs", "--json")
	if code != 0 || !strings.Contains(out, "https://example.com/rmux") {
		t.Fatalf("exit %d: %s %s", code, out, err)
	}
}

func TestRmuxProjectInventoryUsesNativePaneBackend(t *testing.T) {
	f := newFixture(t)
	p := f.repo("work with spaces")
	f.env = append(f.env, "RMUX=/tmp/rmux.sock,42,0", "HERDR_ENV=1")
	f.fake("rmux", fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "list-panes", "-a", "-F", "#{session_id}\t#{session_name}\t#{window_id}\t#{pane_id}\t#{session_attached}\t#{pane_current_command}\t#{pane_active}\t#{pane_pid}\t#{pane_current_path}"}, Output: fmt.Sprintf("$2\tproject\t@3\t%%7\t1\tnu\t1\t10\t%s\n", p)})
	out, err, code := f.run("", "projects", "list", "--json")
	if code != 0 || !strings.Contains(out, `"pane": "%7"`) {
		t.Fatalf("exit %d: %s %s", code, out, err)
	}
}

func TestRmuxPresentationChangesOnlyActiveBackend(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "RMUX=/tmp/rmux.sock,42,0")
	prefix := []string{"-S", "/tmp/rmux.sock"}
	rules := []fakeRule{{Args: append(append([]string{}, prefix...), "list-sessions")}, {Args: append(append([]string{}, prefix...), "show-options", "-gqv", snapshotOption)}}
	for _, item := range presentation {
		rules = append(rules, fakeRule{Args: append(append([]string{}, prefix...), "show-options", "-gqv", item[0]), Output: "original"})
	}
	rules = append(rules, fakeRule{Args: append(append([]string{}, prefix...), "set-option"), Prefix: true})
	f.fake("rmux", rules...)
	_, err, code := f.run("", "tmux-style", "presentation")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, err)
	}
	changed := false
	for _, call := range f.calls() {
		if call.Name != "rmux" {
			t.Fatalf("touched inactive backend: %+v", call)
		}
		if strings.Contains(strings.Join(call.Args, " "), "PRESENTATION") {
			changed = true
		}
	}
	if !changed {
		t.Fatal("presentation did not hide session labels")
	}
}

func TestRmuxSessionInventoryRejectsMalformedIdentities(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "RMUX=/tmp/rmux.sock,42,0")
	f.fake("rmux", fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "list-sessions", "--json"}, Output: `[{"session_id":"-a","session_name":"invalid","session_windows":1}]`},
		fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "list-panes", "-a", "-F", "#{session_id}\t#{pane_current_path}"}})
	_, _, code := f.run("", "sessions", "list", "--json")
	if code == 0 {
		t.Fatal("accepted a command option as a live session id")
	}
}

func TestRmuxProjectDoesNotChooseArbitrarySameDirectorySession(t *testing.T) {
	f := newFixture(t)
	p := f.repo("duplicate")
	f.env = append(f.env, "RMUX=/tmp/rmux.sock,42,0")
	f.fake("rmux", fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "list-sessions", "--json"}, Output: `[{"session_id":"$1","session_name":"one","session_windows":1},{"session_id":"$2","session_name":"two","session_windows":1}]`},
		fakeRule{Args: []string{"-S", "/tmp/rmux.sock", "list-panes", "-a", "-F", "#{session_id}\t#{pane_current_path}"}, Output: "$1\t" + p + "\n$2\t" + p + "\n"})
	f.localEnv()
	if err := rmuxOpenEntry(session{Source: "project", Name: "duplicate", Path: &p}); err == nil || !strings.Contains(err.Error(), "Несколько") {
		t.Fatalf("ambiguous project: %v", err)
	}
}
