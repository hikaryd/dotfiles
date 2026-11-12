package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProjectRegistryWorktreeInventory(t *testing.T) {
	f := newFixture(t)
	main := f.repo("project space")
	linked := filepath.Join(f.base, "linked")
	f.write(filepath.Join(linked, ".git"), "gitdir: "+filepath.Join(main, ".git/worktrees/linked")+"\n", 0600)
	f.write(filepath.Join(main, ".git/worktrees/linked/HEAD"), "ref: refs/heads/feature\n", 0600)
	f.write(filepath.Join(main, ".git/worktrees/linked/commondir"), "../..\n", 0600)
	f.write(filepath.Join(main, ".git/worktrees/linked/gitdir"), filepath.Join(linked, ".git")+"\n", 0600)
	f.write(filepath.Join(main, ".git/worktrees/stale/gitdir"), filepath.Join(f.base, "missing/.git"), 0600)
	f.registry(main)
	panes := "$2\told\t@2\t%2\t0\tzsh\t1\t20\t" + main + "\n$1\tproject space\t@1\t%1\t1\tnvim\t0\t10\t" + main + "\n"
	f.fake("tmux", allRule(panes, 0))
	out, err, code := f.run("", "projects", "list", "--json")
	var projects []project
	if code != 0 || json.Unmarshal([]byte(out), &projects) != nil || len(projects) != 2 {
		t.Fatalf("inventory %d %s %s", code, out, err)
	}
	byPath := map[string]project{}
	for _, p := range projects {
		byPath[p.Path] = p
	}
	if len(byPath[linked].Panes) != 0 || byPath[main].Panes[0].Pane != "%1" || byPath[main].ID != hashID(main, 12) {
		t.Fatalf("wrong inventory %+v", projects)
	}
	for _, c := range f.calls() {
		if c.Name == "git" {
			t.Fatal("list spawned git")
		}
	}
	f.localEnv()
	m := gitMetadata(linked)
	if m.root != linked || m.common != filepath.Join(main, ".git") {
		t.Fatalf("metadata %+v", m)
	}
	if gitMetadata(filepath.Join(main, ".git/objects")).root != "" {
		t.Fatal("git internals treated as project")
	}
}
func TestInvalidRegistryNeverOverwritten(t *testing.T) {
	for _, invalid := range []string{`{"version":true,"projects":[]}`, `{"version":1.0,"projects":[]}`, `{"version":1,"projects":null}`, `{"version":1,"projects":[{"id":"a","path":"relative"}]}`, `{"version":1,"projects":[{"id":"a","path":"/unsafe\npath"}]}`, `broken`} {
		f := newFixture(t)
		path := filepath.Join(f.home, ".config/dots/projects.json")
		f.write(path, invalid, 0600)
		f.fake("tmux", allRule("", 1))
		_, _, code := f.run("", "projects", "refresh")
		if code != 1 {
			t.Errorf("accepted %s", invalid)
		}
		b, _ := os.ReadFile(path)
		if string(b) != invalid {
			t.Fatal("overwrote invalid registry")
		}
	}
}
func TestProjectPathSafety(t *testing.T) {
	f := newFixture(t)
	f.localEnv()
	if allowed(f.home) || allowed(filepath.Join(f.home, ".config")) {
		t.Fatal("private root allowed")
	}
	p := f.repo(".omx/worktree")
	if allowed(p) {
		t.Fatal(".omx allowed")
	}
	cfg := filepath.Join(f.home, ".config/repo")
	f.write(filepath.Join(cfg, ".git/HEAD"), "ref: x", 0600)
	os.MkdirAll(filepath.Join(cfg, ".git/objects"), 0700)
	link := filepath.Join(f.base, "linked-config")
	os.Symlink(cfg, link)
	if allowed(link) {
		t.Fatal("config symlink allowed")
	}
}
func TestProjectOpenFocusAndCreationArgv(t *testing.T) {
	for _, live := range []bool{true, false} {
		t.Run(map[bool]string{true: "focus", false: "new"}[live], func(t *testing.T) {
			f := newFixture(t)
			p := f.repo("a; $(unsafe)")
			f.registry(p)
			f.env = append(f.env, "TMUX=fixture")
			paneOutput := ""
			if live {
				paneOutput = "$1\ta\t@2\t%3\t1\tnvim\t1\t10\t" + p + "\n"
			}
			f.fake("git", fakeRule{Args: []string{"-C", p, "rev-parse", "--show-toplevel"}, Output: p + "\n"})
			f.fake("tmux", fakeRule{Args: []string{"list-panes"}, Prefix: true, Output: paneOutput}, fakeRule{Args: []string{"new-session"}, Prefix: true, Output: "$5\t@6\t%7\n"}, fakeRule{Args: []string{"new-window"}, Prefix: true}, fakeRule{Args: []string{"select-window"}, Prefix: true})
			_, err, code := f.run("", "projects", "open", projectID(p))
			if code != 0 {
				t.Fatalf("open %d %s", code, err)
			}
			calls := f.calls()
			created := 0
			for _, c := range calls {
				if c.Name != "tmux" {
					continue
				}
				if c.Args[0] == "new-session" {
					created++
					if c.Args[len(c.Args)-1] != p {
						t.Fatal("cwd not separate argv")
					}
				}
				if live && has(c.Args[0], "new-window", "new-session") {
					t.Fatal("live session modified")
				}
			}
			if !live && created != 1 {
				t.Fatalf("expected one creation %+v", calls)
			}
			last := calls[len(calls)-1]
			id := "$1"
			paneID, window := "%3", "@2"
			if !live {
				id, paneID, window = "$5", "%7", "@6"
			}
			want := []string{"select-window", "-t", window, ";", "select-pane", "-t", paneID, ";", "switch-client", "-t", id}
			if !reflect.DeepEqual(last.Args, want) {
				t.Fatalf("focus %+v", last.Args)
			}
		})
	}
}
func TestProjectRefreshUsesOneGitQueryPerCommonDir(t *testing.T) {
	f := newFixture(t)
	p := f.repo("main")
	f.registry(p)
	f.fake("tmux", fakeRule{Args: []string{}, Prefix: true, Code: 1, Stderr: "no server running on /fixture/tmux\n"})
	f.fake("git", fakeRule{Args: []string{"-C", p, "worktree", "list", "--porcelain", "-z"}, Output: "worktree " + p + "\x00\x00"})
	_, err, code := f.run("", "projects", "refresh")
	if code != 0 {
		t.Fatal(err)
	}
	calls := f.calls()
	if len(calls) != 2 || calls[1].Name != "git" {
		t.Fatalf("refresh calls %+v", calls)
	}
	info, _ := os.Stat(filepath.Join(f.home, ".config/dots/projects.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("registry not private")
	}
}
func TestSessionIdentityMapping(t *testing.T) {
	f := newFixture(t)
	p := f.repo("project")
	q := f.repo("other")
	key := sessionKey("tmux", "name", p)
	if key != sessionKey("tmux", "name", q) {
		t.Fatal("tmux identity must survive cwd change")
	}
	if sessionKey("config", "name", p) == sessionKey("config", "name", q) {
		t.Fatal("directory identity collision")
	}
}
func TestSessionsCLIInventoryAndFocusPreference(t *testing.T) {
	f := newFixture(t)
	p := f.repo("project")
	f.env = append(f.env, "TMUX=/tmp/tmux-1/default,42,0")
	f.fake("tmux", fakeRule{Args: []string{"list-sessions", "-F", sessionFormat}, Output: "$1\t2\tsame\n"},
		fakeRule{Args: []string{"list-panes", "-a", "-F", panePathFormat}, Output: "$1\t" + p + "\n"},
		fakeRule{Args: []string{"switch-client", "-t", "$1"}})
	rows := []map[string]string{{"Src": "tmux", "Name": "same", "Path": p}, {"Src": "zoxide", "Name": "same", "Path": p}, {"Src": "unknown", "Name": "bad", "Path": p}, {"Src": "zoxide", "Name": "bad\nname", "Path": p}}
	b, _ := json.Marshal(rows)
	f.fake("sesh", fakeRule{Args: []string{"list", "-j"}, Output: string(b)})
	out, err, code := f.run("", "sessions", "list", "--json")
	var entries []session
	if code != 0 || json.Unmarshal([]byte(out), &entries) != nil || len(entries) != 2 {
		t.Fatalf("%d %s %s", code, out, err)
	}
	_, err, code = f.run("", "sessions", "open", "same")
	if code != 0 {
		t.Fatal(err)
	}
	calls := f.calls()
	if !reflect.DeepEqual(calls[len(calls)-1].Args, []string{"switch-client", "-t", "$1"}) {
		t.Fatalf("focus %+v", calls)
	}
	_, err, code = f.run("", "sessions", "open", "missing")
	if code != 1 || !strings.Contains(err, "не найдена") {
		t.Fatalf("bad target %d %s", code, err)
	}
}

func TestSeshSkipsNonObjectRows(t *testing.T) {
	f := newFixture(t)
	p := f.repo("project")
	rows, _ := json.Marshal([]any{"noise", nil, float64(3), map[string]string{"Src": "config", "Name": "valid", "Path": p}})
	f.fake("tmux", fakeRule{Args: []string{"list-sessions"}, Prefix: true, Code: 1, Stderr: "no server running on /tmp/tmux-1/default\n"})
	f.fake("sesh", allRule(string(rows), 0))
	out, err, code := f.run("", "sessions", "list", "--json")
	var entries []session
	if code != 0 || json.Unmarshal([]byte(out), &entries) != nil || len(entries) != 1 || entries[0].Name != "valid" {
		t.Fatalf("sesh filtering %d %s %s", code, out, err)
	}
}
