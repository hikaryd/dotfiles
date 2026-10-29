package rmuxstate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestExecutableDirectoryHomebrewTrustPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, goos, path string
		uid, gid, mode   uint32
		want             bool
	}{
		{"ordinary-user", "linux", "/home/user/bin", 503, 20, 0755, true},
		{"ordinary-root", "darwin", "/usr/bin", 0, 0, 0755, true},
		{"brew-admin", "darwin", "/opt/homebrew/Cellar", 503, 80, 0775, true},
		{"brew-nested", "darwin", "/opt/homebrew/Cellar/rmux/0.10.0", 503, 80, 0775, true},
		{"not-darwin", "linux", "/opt/homebrew/Cellar", 503, 80, 0775, false},
		{"other-group", "darwin", "/opt/homebrew/Cellar", 503, 20, 0775, false},
		{"other-owner", "darwin", "/opt/homebrew/Cellar", 504, 80, 0775, false},
		{"root-group-write", "darwin", "/opt/homebrew/Cellar", 0, 80, 0775, false},
		{"outside-prefix", "darwin", "/opt/homebrew-other/Cellar", 503, 80, 0775, false},
		{"outside-tree", "darwin", "/Users/user/bin", 503, 80, 0775, false},
		{"world-write", "darwin", "/opt/homebrew/Cellar", 503, 80, 0777, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := executableDirectorySafe(tc.goos, tc.path, tc.uid, tc.gid, tc.mode, 503); got != tc.want {
				t.Fatalf("safe=%v want %v", got, tc.want)
			}
		})
	}
}

func TestRealInstalledRmuxExecutableTrust(t *testing.T) {
	bin := os.Getenv("DOTS_RMUX_TEST_BIN")
	if bin == "" {
		t.Skip("set DOTS_RMUX_TEST_BIN for read-only installed executable check")
	}
	if !trustedExecutable(bin) {
		t.Fatalf("installed rmux rejected: %s", bin)
	}
}

func TestJcodeRequiresFixedHomeEntrypoint(t *testing.T) {
	r := Runtime{Home: t.TempDir()}
	if got := r.jcodeBin(); got != "" {
		t.Fatalf("used non-home Jcode entrypoint %q", got)
	}
}

func TestMixedJcodeShellCommandAllowlist(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".local", "bin"), 0700)
	bin := filepath.Join(home, ".local", "bin", "jcode")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0700)
	hist := filepath.Join(home, ".jcode", "sessions")
	os.MkdirAll(hist, 0700)
	os.WriteFile(filepath.Join(hist, "session_safe.json"), []byte("{}"), 0644)
	r := Runtime{Home: home}
	cmd := r.command(Pane{Jcode: "session_safe", StartCommand: "touch /arbitrary"})
	if cmd != shellQuote(bin)+" --no-update --resume session_safe" {
		t.Fatalf("allowlist command %q", cmd)
	}
	for _, p := range []Pane{{StartCommand: "touch /arbitrary"}, {Jcode: "session_missing"}, {Jcode: "session_bad;touch"}} {
		cmd = r.command(p)
		if strings.Contains(cmd, "arbitrary") || strings.Contains(cmd, "--resume") {
			t.Fatal(cmd)
		}
	}
}
func TestRealIsolatedRmuxRoundtrip(t *testing.T) {
	bin := os.Getenv("DOTS_RMUX_TEST_BIN")
	if bin == "" {
		t.Skip("set DOTS_RMUX_TEST_BIN for isolated native acceptance")
	}
	base, e := os.MkdirTemp(os.Getenv("JCODE_SCRATCH_DIR"), "rs-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(base)
	home := filepath.Join(base, "home")
	os.Mkdir(home, 0700)
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("RMUX_PANE", "%999")
	t.Setenv("TMUX", "unrelated")
	config := filepath.Join(home, "rmux.conf")
	os.WriteFile(config, []byte("set -g default-shell /bin/sh\nset -g base-index 1\nset -g pane-base-index 1\nset -g renumber-windows on\nset -g status off\n"), 0600)
	s, e := Open(filepath.Join(home, "persist"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r := Runtime{Home: home, Bin: bin, Socket: filepath.Join(base, "s"), Config: config, Store: s, Warnings: os.Stderr}
	call := func(args ...string) string {
		t.Helper()
		v, e := r.call(args...)
		if e != nil {
			t.Fatal(e)
		}
		return strings.TrimSpace(v)
	}
	defer r.call("kill-server")
	call("-f", config, "new-session", "-d", "-s", "decoy", "/bin/sh -l")
	call("new-session", "-d", "-s", "work", "-n", "code", "-x", "128", "-y", "48", "-c", home, "/bin/sh -l")
	call("kill-session", "-t", "decoy")
	call("move-window", "-s", "work:1", "-t", "work:2")
	first := call("display-message", "-p", "-t", "work:2", "#{pane_id}")
	call("split-window", "-d", "-h", "-t", first, "-c", "/", "/bin/sh -l")
	call("split-window", "-d", "-v", "-t", first, "-c", home, "/bin/sh -l")
	call("new-window", "-d", "-t", "work:7", "-n", "logs", "-c", "/", "/bin/sh -l")
	call("select-pane", "-t", first)
	call("select-window", "-t", "work:7")
	// Let shells settle before the consistency-checked snapshot.
	time.Sleep(150 * time.Millisecond)
	changed, e := r.Save()
	if e != nil || !changed {
		t.Fatalf("initial save %v %v", changed, e)
	}
	before, e := s.Load()
	if e != nil {
		t.Fatal(e)
	}
	call("kill-server")
	if e = r.Restore(); e != nil {
		t.Fatal(e)
	}
	time.Sleep(150 * time.Millisecond)
	after, e := r.Capture()
	if e != nil {
		t.Fatal(e)
	}
	normal := regexp.MustCompile(`([0-9]+x[0-9]+,[0-9]+,[0-9]+),[0-9]+`)
	if len(before.Sessions) != len(after.Sessions) {
		t.Fatal("session count")
	}
	differentID := false
	for si, se := range before.Sessions {
		got := after.Sessions[si]
		if se.Name != got.Name || len(se.Windows) != len(got.Windows) {
			t.Fatal("sessions/windows changed")
		}
		for wi, w := range se.Windows {
			g := got.Windows[wi]
			if w.Index != g.Index || w.Name != g.Name || w.Active != g.Active || w.Width != g.Width || w.Height != g.Height || normal.ReplaceAllString(w.Layout[5:], "${1},ID") != normal.ReplaceAllString(g.Layout[5:], "${1},ID") {
				t.Fatalf("layout/focus/index mismatch\n before=%+v\n after=%+v", w, g)
			}
			if len(w.Panes) != len(g.Panes) {
				t.Fatal("pane count")
			}
			for pi, p := range w.Panes {
				gp := g.Panes[pi]
				if p.ID != gp.ID {
					differentID = true
				}
				if p.Index != gp.Index || p.CWD != gp.CWD || p.Active != gp.Active {
					t.Fatalf("pane geometry/order/focus mismatch %v %v", p, gp)
				}
			}
		}
	}
	if !differentID {
		t.Fatal("fixture failed to exercise old/new pane IDs")
	}
	if e = r.Restore(); e != nil {
		t.Fatal(e)
	}
	if !r.ready() {
		t.Fatal("complete restore not ready")
	}
	t.Log(fmt.Sprintf("native roundtrip preserved nested geometry, focus, cwd, window indices2/7 and changed pane IDs on %s", r.Socket))
}
