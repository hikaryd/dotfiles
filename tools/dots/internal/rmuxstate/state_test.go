package rmuxstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture() Snapshot {
	return Snapshot{Version: 1, Sessions: []Session{{Name: "work", Windows: []Window{{Index: 2, Name: "editor", Layout: "b264,80x24,0,0,7", Width: 80, Height: 24, Active: true, Panes: []Pane{{ID: "%7", Index: 0, CWD: "/", Active: true}}}}}}}
}
func storeFor(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestStateAtomicPrivateChangedOnly(t *testing.T) {
	s := storeFor(t)
	snap := fixture()
	changed, err := s.Save(snap)
	if err != nil || !changed {
		t.Fatalf("save %v %v", changed, err)
	}
	before, err := os.ReadFile(filepath.Join(s.Dir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	changed, err = s.Save(snap)
	if err != nil || changed {
		t.Fatalf("unchanged %v %v", changed, err)
	}
	snap.Sessions = nil
	if _, err = s.Save(snap); err == nil {
		t.Fatal("accepted empty snapshot")
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "latest"))
	if string(before) != string(after) {
		t.Fatal("last good changed")
	}
	loaded, err := s.Load()
	if err != nil || loaded.Sessions[0].Windows[0].Index != 2 {
		t.Fatalf("load %v %v", loaded, err)
	}
	entries, _ := os.ReadDir(s.Dir)
	for _, e := range entries {
		i, _ := e.Info()
		if i.Mode().Perm() != 0600 {
			t.Fatalf("mode %s: %v", e.Name(), i.Mode())
		}
	}
	i, _ := os.Stat(s.Dir)
	if i.Mode().Perm() != 0700 {
		t.Fatal(i.Mode())
	}
}
func TestStateRejectsUnsafePathsAndFiles(t *testing.T) {
	for _, kind := range []string{"symlink-dir", "public-dir", "symlink-latest", "public-latest", "corrupt", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "state")
			if kind == "symlink-dir" {
				os.Symlink(base, dir)
				if s, e := Open(dir); e == nil {
					s.Close()
					t.Fatal("accepted symlink")
				}
				return
			}
			if kind == "public-dir" {
				os.Mkdir(dir, 0755)
				if e := os.Chmod(dir, 0755); e != nil {
					t.Fatal(e)
				}
				if s, e := Open(dir); e == nil {
					s.Close()
					t.Fatal("accepted public state")
				}
				return
			}
			s, e := Open(dir)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			if _, e = s.Save(fixture()); e != nil {
				t.Fatal(e)
			}
			latest := filepath.Join(dir, "latest")
			switch kind {
			case "symlink-latest":
				os.Remove(latest)
				os.Symlink("elsewhere", latest)
			case "public-latest":
				os.Chmod(latest, 0644)
			case "corrupt":
				os.WriteFile(latest, []byte("../escape"), 0600)
			case "hardlink":
				os.Link(latest, filepath.Join(dir, "alias"))
			}
			if _, e = s.Load(); e == nil {
				t.Fatal("accepted unsafe latest")
			}
			if _, e = s.Save(fixture()); e == nil {
				t.Fatal("overwrote unsafe latest")
			}
		})
	}
}
func TestStrictSnapshotValidation(t *testing.T) {
	for _, mutate := range []func(*Snapshot){func(s *Snapshot) { s.Version = 2 }, func(s *Snapshot) { s.Sessions[0].Name = "bad\nname" }, func(s *Snapshot) { s.Sessions[0].Name = "-flag" }, func(s *Snapshot) { s.Sessions[0].Windows[0].Panes[0].Jcode = "session_bad;touch" }, func(s *Snapshot) { s.Sessions[0].Windows[0].Panes[0].CWD = "relative" }, func(s *Snapshot) { s.Sessions[0].Windows[0].Layout = "bad" }} {
		s := fixture()
		mutate(&s)
		if e := s.Validate(); e == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
}
func TestLiveDescendantSessionMapping(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".jcode", "client_sessions")
	os.MkdirAll(dir, 0700)
	histories := filepath.Join(home, ".jcode", "sessions")
	os.MkdirAll(histories, 0700)
	for pid, id := range map[string]string{"12": "session_right", "90": "session_stale", "22": "session_unrelated"} {
		os.WriteFile(filepath.Join(dir, pid), []byte(id), 0600)
		os.WriteFile(filepath.Join(histories, id+".json"), []byte("{}"), 0600)
	}
	exe := filepath.Join(home, "jcode")
	os.WriteFile(exe, []byte("#!/bin/sh\n"), 0700)
	processes := map[int]Process{10: {Parent: 1}, 11: {Parent: 10}, 12: {Parent: 11, Executable: exe, Started: time.Now().Add(-time.Minute)}, 22: {Parent: 1, Executable: exe, Started: time.Now().Add(-time.Minute)}}
	if got := ResolveJcode(home, 10, processes); got != "session_right" {
		t.Fatal(got)
	}
	delete(processes, 12)
	if got := ResolveJcode(home, 10, processes); got != "" {
		t.Fatalf("stale mapping %q", got)
	}
	processes[12] = Process{Parent: 11, Executable: exe, Started: time.Now().Add(-time.Minute)}
	processes[11] = Process{Parent: 10, Executable: exe, Started: time.Now().Add(-time.Minute)}
	os.WriteFile(filepath.Join(dir, "11"), []byte("session_unrelated"), 0600)
	if got := ResolveJcode(home, 10, processes); got != "" {
		t.Fatalf("ambiguous mapping %q", got)
	}
}
func TestLayoutLeafOrderAndChecksum(t *testing.T) {
	body := "120x40,0,0{59x40,0,0,19,60x40,60,0[60x19,60,0,4,60x20,60,20,33]}"
	layout := withChecksum(body)
	ids, err := layoutIDs(layout)
	if err != nil || strings.Join(ids, ",") != "%19,%4,%33" {
		t.Fatalf("%v %v", ids, err)
	}
	if _, e := layoutIDs("0000," + body); e == nil {
		t.Fatal("bad checksum accepted")
	}
}

func TestStateMustRemainOutsideGit(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			git := filepath.Join(root, ".git")
			switch kind {
			case "file":
				os.WriteFile(git, []byte("gitdir: elsewhere"), 0600)
			case "directory":
				os.Mkdir(git, 0700)
			case "symlink":
				os.Symlink("missing", git)
			}
			s, e := Open(filepath.Join(root, "nested", "state"))
			if e == nil {
				s.Close()
				t.Fatal("created persistence inside Git")
			}
		})
	}
}

func TestMappingRejectsPIDReuseAndNonJcodeButAcceptsDefault0644(t *testing.T) {
	home := t.TempDir()
	maps := filepath.Join(home, ".jcode", "client_sessions")
	history := filepath.Join(home, ".jcode", "sessions")
	os.MkdirAll(maps, 0700)
	os.MkdirAll(history, 0700)
	exe := filepath.Join(home, "jcode")
	os.WriteFile(exe, []byte("#!/bin/sh\n"), 0700)
	mapping := filepath.Join(maps, "44")
	os.WriteFile(mapping, []byte("session_exact"), 0644)
	os.WriteFile(filepath.Join(history, "session_exact.json"), []byte("{}"), 0644)
	processes := map[int]Process{40: {Parent: 1}, 44: {Parent: 40, Executable: exe, Started: time.Now().Add(-time.Minute)}}
	if got := ResolveJcode(home, 40, processes); got != "session_exact" {
		t.Fatalf("default Jcode metadata omitted %q", got)
	}
	justBefore := processes[44].Started.Add(-500 * time.Millisecond)
	if e := os.Chtimes(mapping, justBefore, justBefore); e != nil {
		t.Fatal(e)
	}
	if got := ResolveJcode(home, 40, processes); got != "" {
		t.Fatalf("mapping before process start accepted %q", got)
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(mapping, old, old)
	if got := ResolveJcode(home, 40, processes); got != "" {
		t.Fatalf("reused PID %q", got)
	}
	os.Chtimes(mapping, time.Now(), time.Now())
	p := processes[44]
	p.Executable = "/bin/sh"
	processes[44] = p
	if got := ResolveJcode(home, 40, processes); got != "" {
		t.Fatalf("non-Jcode descendant %q", got)
	}
}

func TestOversizeSnapshotDoesNotReplaceLastGood(t *testing.T) {
	s := storeFor(t)
	s.Save(fixture())
	before, _ := s.read("latest")
	v := Snapshot{Version: 1, Sessions: []Session{{Name: "large"}}}
	for w := 0; w < 6; w++ {
		window := Window{Index: w, Name: "large", Width: 1000, Height: 40, Active: w == 0}
		var leaves []string
		for p := 0; p < 100; p++ {
			id := w*100 + p
			window.Panes = append(window.Panes, Pane{ID: fmt.Sprintf("%%%d", id), Index: p, CWD: "/", Active: p == 0, StartCommand: strings.Repeat("x", 16384)})
			leaves = append(leaves, fmt.Sprintf("9x40,%d,0,%d", p*10, id))
		}
		window.Layout = withChecksum("1000x40,0,0{" + strings.Join(leaves, ",") + "}")
		v.Sessions[0].Windows = append(v.Sessions[0].Windows, window)
	}
	if _, e := s.Save(v); e == nil {
		t.Fatal("saved unreadable oversized snapshot")
	}
	after, _ := s.read("latest")
	if string(before) != string(after) {
		t.Fatal("oversize replaced last good")
	}
	if _, e := s.Load(); e != nil {
		t.Fatal(e)
	}
}

func TestRetentionBoundsOnlyValidatedOwnedSnapshots(t *testing.T) {
	s := storeFor(t)
	unrelated := "snapshot-" + strings.Repeat("a", 64) + ".json"
	os.WriteFile(filepath.Join(s.Dir, unrelated), []byte("unrelated"), 0600)
	for n := 0; n < 64; n++ {
		v := fixture()
		v.Sessions[0].Windows[0].Panes[0].Command = fmt.Sprintf("reference%d", n)
		if _, e := s.Save(v); e != nil {
			t.Fatal(e)
		}
	}
	entries, e := os.ReadDir(s.Dir)
	if e != nil {
		t.Fatal(e)
	}
	valid := 0
	for _, de := range entries {
		if snapshotName.MatchString(de.Name()) && de.Name() != unrelated {
			valid++
		}
	}
	if valid != 61 {
		t.Fatalf("retained %d snapshots, want latest plus60 prior", valid)
	}
	b, e := os.ReadFile(filepath.Join(s.Dir, unrelated))
	if e != nil || string(b) != "unrelated" {
		t.Fatal("retention deleted unrelated path")
	}
	v, e := s.Load()
	if e != nil || v.Sessions[0].Windows[0].Panes[0].Command != "reference63" {
		t.Fatalf("latest %v %v", v, e)
	}
}
