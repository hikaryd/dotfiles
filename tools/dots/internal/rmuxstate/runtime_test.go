package rmuxstate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveGuardPreservesLastGood(t *testing.T) {
	s := storeFor(t)
	if _, e := s.Save(fixture()); e != nil {
		t.Fatal(e)
	}
	before, _ := s.read("latest")
	for _, mode := range []string{"unmarked", "failed-query", "empty", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			r := Runtime{Home: t.TempDir(), Store: s, run: func(a ...string) (string, error) {
				switch a[0] {
				case "show-options":
					return "", nil
				case "list-sessions":
					if mode == "failed-query" {
						return "", errors.New("permission denied")
					}
					if mode == "empty" {
						return "[]", nil
					}
					if mode == "malformed" {
						return "oops", nil
					}
					return `[{"session_name":"unrelated"}]`, nil
				}
				return "", fmt.Errorf("unexpected command %v", a)
			}}
			if _, e := r.Save(); e == nil {
				t.Fatal("unsafe save succeeded")
			}
			after, _ := s.read("latest")
			if !bytes.Equal(before, after) {
				t.Fatal("last good changed")
			}
		})
	}
}
func TestRestoreRefusesRunningUnrelatedAndCorruptState(t *testing.T) {
	s := storeFor(t)
	s.Save(fixture())
	mutations := 0
	r := Runtime{Home: t.TempDir(), Store: s, run: func(a ...string) (string, error) {
		switch a[0] {
		case "show-options":
			return "", nil
		case "list-sessions":
			return `[{"session_name":"unrelated"}]`, nil
		}
		mutations++
		return "", nil
	}}
	if e := r.Restore(); e == nil {
		t.Fatal("restored into unrelated server")
	}
	if mutations != 0 {
		t.Fatal("mutated unrelated server")
	}
	os.WriteFile(filepath.Join(s.Dir, "latest"), []byte("garbage"), 0600)
	if e := r.Restore(); e == nil {
		t.Fatal("accepted corrupt state")
	}
	if mutations != 0 {
		t.Fatal("mutated on corrupt state")
	}
}
func TestRestoreShellOnlyAndPartialGuard(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			s := storeFor(t)
			snap := fixture()
			snap.Sessions[0].Windows[0].Panes[0].StartCommand = "touch /must-not-run"
			snap.Sessions[0].Windows[0].Panes[0].CWD = "/missing/dots-rmux-test"
			s.Save(snap)
			var calls [][]string
			ready := ""
			var warning bytes.Buffer
			r := Runtime{Home: t.TempDir(), Store: s, Warnings: &warning, run: func(a ...string) (string, error) {
				calls = append(calls, append([]string(nil), a...))
				switch a[0] {
				case "show-options":
					if ready != "" {
						return ready, nil
					}
					return "", nil
				case "list-sessions":
					return "", ErrNoServer
				case "new-session":
					if fail {
						return "", errors.New("injected create failure")
					}
					return "%81", nil
				case "list-windows":
					return `[{"session_name":"work","window_index":0}]`, nil
				case "set-option":
					if a[len(a)-2] == readyOption {
						ready = a[len(a)-1]
					}
				}
				return "", nil
			}}
			e := r.Restore()
			if fail {
				if e == nil || ready != "" {
					t.Fatal("partial restore marked ready")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if ready == "" || !strings.Contains(warning.String(), "cwd") {
				t.Fatalf("ready=%v warning=%q", ready, warning.String())
			}
			for _, c := range calls {
				for _, a := range c {
					if strings.Contains(a, "must-not-run") {
						t.Fatal("executed saved command")
					}
				}
			}
			before := len(calls)
			if e = r.Restore(); e != nil {
				t.Fatal(e)
			}
			for _, c := range calls[before:] {
				if c[0] != "show-options" {
					t.Fatalf("idempotent restore mutated %v", c)
				}
			}
		})
	}
}
func TestExclusiveStartupLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	first, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	ch := make(chan *Store, 1)
	go func() {
		second, e := Open(dir)
		if e != nil {
			ch <- nil
			return
		}
		ch <- second
	}()
	select {
	case s := <-ch:
		if s != nil {
			s.Close()
		}
		t.Fatal("second startup bypassed lock")
	case <-time.After(40 * time.Millisecond):
	}
	first.Close()
	select {
	case s := <-ch:
		if s == nil {
			t.Fatal("second lock failed")
		}
		s.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("lock not released")
	}
}

func TestCaptureFailsClosedWhenProcessCollectionFails(t *testing.T) {
	s := storeFor(t)
	s.Save(fixture())
	r := Runtime{Store: s, processes: func() (map[int]Process, error) { return nil, errors.New("ps failed") }, run: func(a ...string) (string, error) { return "[]", nil }}
	if _, e := r.Capture(); e == nil || !strings.Contains(e.Error(), "ps failed") {
		t.Fatalf("lost process collection error %v", e)
	}
}

func TestCaptureRejectsReusedPanePIDAndMissingJcodeMapping(t *testing.T) {
	for _, mode := range []string{"pid-reused", "missing-jcode"} {
		t.Run(mode, func(t *testing.T) {
			queries := 0
			started := time.Now().Add(-time.Minute)
			r := Runtime{Home: t.TempDir(), processes: func() (map[int]Process, error) {
				queries++
				p := Process{Parent: 1, Started: started, Executable: "/bin/sh"}
				if mode == "pid-reused" && queries > 1 {
					p.Started = time.Now()
				}
				if mode == "missing-jcode" {
					p.Executable = "/missing/jcode"
				}
				return map[int]Process{100: p}, nil
			}, run: func(a ...string) (string, error) {
				switch a[0] {
				case "list-sessions":
					return `[{"session_name":"work"}]`, nil
				case "list-windows":
					return `[{"session_name":"work","window_index":2,"window_name":"editor","window_layout":"b264,80x24,0,0,7","window_width":80,"window_height":24,"window_active":true}]`, nil
				case "list-panes":
					return `[{"session_name":"work","window_index":2,"pane_index":0,"pane_id":"%7","pane_current_path":"/","pane_active":true}]`, nil
				case "display-message":
					if a[len(a)-1] == "#{pane_pid}" {
						return "100", nil
					}
					return "", nil
				}
				return "", errors.New("unexpected call")
			}}
			if _, e := r.Capture(); e == nil {
				t.Fatal("unsafe process identity captured")
			}
		})
	}
}
