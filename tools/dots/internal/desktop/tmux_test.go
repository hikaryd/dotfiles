package desktop

import (
	"encoding/json"
	"strings"
	"testing"
)

const sessionFormat = "#{session_id}\t#{session_windows}\t#{session_name}"
const panePathFormat = "#{session_id}\t#{pane_current_path}"

func TestTmuxSessionsListLiveWithoutDuplicatingSesh(t *testing.T) {
	f := newFixture(t)
	f.fake("tmux", fakeRule{Args: []string{"list-sessions", "-F", sessionFormat}, Output: "$2\t3\tproject with spaces\n"},
		fakeRule{Args: []string{"list-panes", "-a", "-F", panePathFormat}, Output: "$2\t" + f.root + "\n"})
	b, _ := json.Marshal([]map[string]string{{"Src": "tmux", "Name": "project with spaces", "Path": f.root}})
	f.fake("sesh", fakeRule{Args: []string{"list", "-j"}, Output: string(b)})
	out, err, code := f.run("", "sessions", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, err)
	}
	var rows []session
	if json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 1 || rows[0].Source != "tmux" || rows[0].ID != "tmux:$2" || *rows[0].Tabs != 3 || ptrText(rows[0].Path) != f.root {
		t.Fatalf("inventory=%s", out)
	}
}

func TestTmuxNoServerIsAnEmptyInventory(t *testing.T) {
	f := newFixture(t)
	f.fake("tmux", fakeRule{Args: []string{"list-sessions"}, Prefix: true, Code: 1, Stderr: "error connecting to /private/tmp/tmux-503/default (No such file or directory)\n"})
	out, err, code := f.run("", "sessions", "list", "--json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("exit %d: %q %s", code, out, err)
	}
}

func TestTmuxSessionOpenTargetsPopupClientAndDoesNotRecreate(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "TMUX=/tmp/tmux-1/default,42,0", "DOTS_MUX_CLIENT=/dev/ttys017")
	f.fake("tmux", fakeRule{Args: []string{"list-sessions", "-F", sessionFormat}, Output: "$2\t1\texisting\n"},
		fakeRule{Args: []string{"list-panes", "-a", "-F", panePathFormat}, Output: "$2\t" + f.root + "\n"},
		fakeRule{Args: []string{"switch-client", "-c", "/dev/ttys017", "-t", "$2"}})
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

func TestTmuxProjectSessionIsCreatedOnce(t *testing.T) {
	f := newFixture(t)
	p := f.repo("fresh")
	f.env = append(f.env, "TMUX=/tmp/tmux-1/default,42,0")
	f.fake("tmux", fakeRule{Args: []string{"list-sessions", "-F", sessionFormat}, Output: ""},
		fakeRule{Args: []string{"list-panes", "-a", "-F", panePathFormat}, Output: ""},
		fakeRule{Args: []string{"new-session", "-d", "-P", "-F", "#{session_id}", "-s", sessionName(p), "-c", p}, Output: "$7\n"},
		fakeRule{Args: []string{"switch-client", "-t", "$7"}})
	f.localEnv()
	t.Setenv("TMUX", "/tmp/tmux-1/default,42,0")
	if err := openEntry(session{Source: "project", Name: "fresh", Path: &p}); err != nil {
		t.Fatal(err)
	}
}

func TestTmuxSessionInventoryRejectsMalformedIdentities(t *testing.T) {
	f := newFixture(t)
	f.fake("tmux", fakeRule{Args: []string{"list-sessions", "-F", sessionFormat}, Output: "-a\t1\tinvalid\n"},
		fakeRule{Args: []string{"list-panes", "-a", "-F", panePathFormat}})
	_, _, code := f.run("", "sessions", "list", "--json")
	if code == 0 {
		t.Fatal("accepted a command option as a live session id")
	}
}

func TestTmuxProjectDoesNotChooseArbitrarySameDirectorySession(t *testing.T) {
	f := newFixture(t)
	p := f.repo("duplicate")
	f.fake("tmux", fakeRule{Args: []string{"list-sessions", "-F", sessionFormat}, Output: "$1\t1\tone\n$2\t1\ttwo\n"},
		fakeRule{Args: []string{"list-panes", "-a", "-F", panePathFormat}, Output: "$1\t" + p + "\n$2\t" + p + "\n"})
	f.localEnv()
	if err := openEntry(session{Source: "project", Name: "duplicate", Path: &p}); err == nil || !strings.Contains(err.Error(), "Несколько") {
		t.Fatalf("ambiguous project: %v", err)
	}
}
