package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
