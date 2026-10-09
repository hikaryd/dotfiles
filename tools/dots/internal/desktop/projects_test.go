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
	panes := "$2\told\t@2\t%2\t0\tzsh\t1\t" + main + "\n$1\tproject space\t@1\t%1\t1\tnvim\t0\t" + main + "\n"
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
				paneOutput = "$1\ta\t@2\t%3\t1\tnvim\t1\t" + p + "\n"
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
	f.fake("tmux", allRule("", 1))
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
	snapshot := map[string]any{"workspaces": []any{map[string]any{"workspace_id": "w1", "label": "name", "tokens": map[string]any{"dots_session_id": key}}}, "panes": []any{map[string]any{"workspace_id": "w1", "cwd": q}}}
	entry := session{Source: "tmux", Name: "name", Path: &p, ImportKey: &key}
	id, e := existingWorkspace(entry, snapshot, nil)
	if e != nil || id != "w1" {
		t.Fatalf("mapping %s %v", id, e)
	}
	snapshot["workspaces"] = append(array(snapshot["workspaces"]), map[string]any{"workspace_id": "w2", "label": "name", "tokens": map[string]any{"dots_session_id": key}})
	if _, e = existingWorkspace(entry, snapshot, nil); e == nil {
		t.Fatal("ambiguous imports accepted")
	}
	entry.ImportKey = nil
	snapshot = map[string]any{"workspaces": []any{map[string]any{"workspace_id": "w3", "label": "name"}}, "panes": []any{}, "tabs": []any{map[string]any{"workspace_id": "w3", "number": 2, "label": "logs", "pane_count": float64(1)}, map[string]any{"workspace_id": "w3", "number": 1, "label": "editor", "pane_count": float64(2)}}}
	id, e = existingWorkspace(entry, snapshot, []layoutWindow{{label: "editor", panes: make([]layoutPane, 2)}, {label: "logs", panes: make([]layoutPane, 1)}})
	if e != nil || id != "w3" {
		t.Fatalf("layout mapping %s %v", id, e)
	}
}
func TestSessionsCLIInventoryAndFocusPreference(t *testing.T) {
	f := newFixture(t)
	p := f.repo("project")
	snapshot := map[string]any{"result": map[string]any{"snapshot": map[string]any{"workspaces": []any{map[string]any{"workspace_id": "w1", "label": "same", "tab_count": 2, "pane_count": 3}}, "panes": []any{map[string]any{"workspace_id": "w1", "cwd": p}}}}}
	b, _ := json.Marshal(snapshot)
	f.fake("herdr", fakeRule{Args: []string{"api", "snapshot"}, Output: string(b)}, fakeRule{Args: []string{"workspace", "focus", "w1"}, Output: `{"result":{}}`})
	rows := []map[string]string{{"Src": "tmux", "Name": "same", "Path": p}, {"Src": "tmux", "Name": "same", "Path": p}, {"Src": "unknown", "Name": "bad", "Path": p}, {"Src": "zoxide", "Name": "bad\nname", "Path": p}}
	b, _ = json.Marshal(rows)
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
	if !reflect.DeepEqual(calls[len(calls)-1].Args, []string{"workspace", "focus", "w1"}) {
		t.Fatalf("focus %+v", calls)
	}
	_, err, code = f.run("", "sessions", "open", "missing")
	if code != 1 || !strings.Contains(err, "не найдена") {
		t.Fatalf("bad target %d %s", code, err)
	}
}
func TestSessionsImportFreshLayout(t *testing.T) {
	f := newFixture(t)
	p := f.repo("project")
	b, _ := json.Marshal([]map[string]string{{"Src": "tmux", "Name": "fixture", "Path": p}})
	f.fake("sesh", allRule(string(b), 0))
	f.fake("tmux", fakeRule{Args: []string{"list-windows"}, Prefix: true, Output: "fixture\t@2\t2\tlogs\nfixture\t@1\t1\teditor\n"}, fakeRule{Args: []string{"list-panes"}, Prefix: true, Output: "fixture\t@1\t1\t" + p + "\nfixture\t@1\t0\t" + p + "\nfixture\t@2\t0\t/missing-fixture-path\n"})
	f.fake("herdr", fakeRule{Args: []string{"api", "snapshot"}, Output: `{"result":{"snapshot":{"workspaces":[],"panes":[]}}}`}, fakeRule{Args: []string{"workspace", "create"}, Prefix: true, Output: `{"result":{"workspace":{"workspace_id":"w1"},"tab":{"tab_id":"w1:t1"},"root_pane":{"pane_id":"w1:p1"}}}`}, fakeRule{Args: []string{"workspace", "report-metadata"}, Prefix: true}, fakeRule{Args: []string{"tab", "rename"}, Prefix: true, Output: `{"result":{}}`}, fakeRule{Args: []string{"pane", "split"}, Prefix: true, Output: `{"result":{}}`}, fakeRule{Args: []string{"tab", "create"}, Prefix: true, Output: `{"result":{"root_pane":{"pane_id":"w1:p3"}}}`}, fakeRule{Args: []string{"workspace", "focus"}, Prefix: true, Output: `{"result":{}}`})
	out, err, code := f.run("", "sessions", "open", "fixture")
	if code != 0 || !strings.Contains(out, "процессы tmux не переносились") {
		t.Fatalf("import %d %q %q", code, out, err)
	}
	calls := f.calls()
	split, metadata, tab := false, false, false
	for _, c := range calls {
		if c.Name == "tmux" && !has(c.Args[0], "list-panes", "list-windows") {
			t.Fatal("mutated tmux")
		}
		if c.Name == "herdr" {
			if len(c.Args) > 1 && c.Args[0] == "pane" && c.Args[1] == "split" {
				split = true
				if !reflect.DeepEqual(c.Args, []string{"pane", "split", "w1:p1", "--direction", "right", "--cwd", p, "--no-focus"}) {
					t.Fatal(c.Args)
				}
			}
			if len(c.Args) > 1 && c.Args[1] == "report-metadata" {
				metadata = true
			}
			if len(c.Args) > 1 && c.Args[0] == "tab" && c.Args[1] == "create" {
				tab = true
				if !has(p, c.Args...) {
					t.Fatal("missing fallback cwd")
				}
			}
		}
	}
	if !split || !metadata || !tab {
		t.Fatalf("missing import operation %+v", calls)
	}
}

func TestSeshSkipsNonObjectRows(t *testing.T) {
	f := newFixture(t)
	p := f.repo("project")
	rows, _ := json.Marshal([]any{"noise", nil, float64(3), map[string]string{"Src": "config", "Name": "valid", "Path": p}})
	f.fake("herdr", allRule(`{"result":{"snapshot":{"workspaces":[],"panes":[]}}}`, 0))
	f.fake("sesh", allRule(string(rows), 0))
	out, err, code := f.run("", "sessions", "list", "--json")
	var entries []session
	if code != 0 || json.Unmarshal([]byte(out), &entries) != nil || len(entries) != 1 || entries[0].Name != "valid" {
		t.Fatalf("sesh filtering %d %s %s", code, out, err)
	}
}
