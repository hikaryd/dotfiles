package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCanceledCollectionDoesNotPublishPartialStatus(t *testing.T) {
	f := newFixture(t)
	paths := []string{}
	for i := 0; i < 9; i++ {
		paths = append(paths, f.repo(fmt.Sprintf("project-%d", i)))
	}
	f.registry(paths...)
	f.fake("tmux", allRule("", 0))
	f.fake("ps", allRule("", 0))
	f.fake("lsof", allRule("", 0))
	f.fake("git", fakeRule{Args: []string{}, Prefix: true, Block: true})
	f.localEnv()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := collectStatus(ctx, true); result <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for !exists(filepath.Join(f.bin, "blocked-git")) {
		if time.Now().After(deadline) {
			t.Fatal("worker did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("partial success: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation blocked")
	}
}

func TestMissingGitStateIsUnknown(t *testing.T) {
	if got := gitSummary(gitState{}); got != "? unknown" {
		t.Fatal(got)
	}
}

func TestFailedDiscoveryPreservesRegistry(t *testing.T) {
	for _, tool := range []string{"tmux", "git"} {
		t.Run(tool, func(t *testing.T) {
			f := newFixture(t)
			p := f.repo("project")
			f.registry(p)
			f.fake("tmux", allRule("", 0))
			f.fake(tool, allRule("", 2))
			path := filepath.Join(f.home, ".config/dots/projects.json")
			before, _ := os.ReadFile(path)
			_, _, code := f.run("", "projects", "refresh")
			if code == 0 {
				t.Fatal("failed discovery reported success")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(before) {
				t.Fatal("registry changed after failed discovery")
			}
		})
	}
}

func TestIncompleteWorkspaceCannotMatchFallback(t *testing.T) {
	f := newFixture(t)
	path := f.root
	key := "fixture-import"
	entry := session{Source: "tmux", Name: "project", Path: &path, ImportKey: &key}
	snapshot := map[string]any{"workspaces": []any{map[string]any{"workspace_id": "w1", "label": "project", "tokens": map[string]any{"dots_session_id": "pending:" + key}}}, "panes": []any{map[string]any{"workspace_id": "w1", "cwd": path}}}
	id, err := existingWorkspace(entry, snapshot, nil)
	if err == nil || id != "" {
		t.Fatalf("incomplete workspace accepted: %q %v", id, err)
	}
}

func TestUnmarkedPartialWorkspaceCannotMatchFallback(t *testing.T) {
	path := t.TempDir()
	entry := session{Source: "tmux", Name: "project", Path: &path}
	snapshot := map[string]any{"workspaces": []any{map[string]any{"workspace_id": "w1", "label": "project"}}, "panes": []any{map[string]any{"workspace_id": "w1", "cwd": path}}, "tabs": []any{map[string]any{"workspace_id": "w1", "number": 1, "label": "first", "pane_count": 1}}}
	windows := []layoutWindow{{label: "first", panes: []layoutPane{{}}}, {label: "second", panes: []layoutPane{{}}}}
	if id, err := existingWorkspace(entry, snapshot, windows); err == nil || id != "" {
		t.Fatalf("partial reuse: %q %v", id, err)
	}
	windows = windows[:1]
	if id, err := existingWorkspace(entry, snapshot, windows); err != nil || id != "w1" {
		t.Fatalf("complete reuse: %q %v", id, err)
	}
}

func TestNuCacheFollowsRelocatedRoot(t *testing.T) {
	f := newFixture(t)
	f.fake("nu", allRule("NU\n", 0))
	f.fake("dots-zsh", allRule("hello\n", 0))
	f.localEnv()
	os.MkdirAll(filepath.Join(f.root, "scripts"), 0700)
	os.Symlink(filepath.Join(f.bin, "dots-zsh"), filepath.Join(f.root, "scripts/dots-zsh"))
	if _, _, _, err := prepareNu(f.root, nil); err != nil {
		t.Fatal(err)
	}
	old := f.root
	next := old + "-moved"
	if err := os.Rename(old, next); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(f.home, ".cache/dots/zsh-bridge/bin")
	os.Symlink("/foreign/missing", filepath.Join(cache, "foreign"))
	if _, _, _, err := prepareNu(next, nil); err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(cache, "hello"))
	if err != nil || !strings.HasPrefix(link, next+"/") {
		t.Fatalf("old bridge retained: %q %v", link, err)
	}
	link, err = os.Readlink(filepath.Join(cache, "foreign"))
	if err != nil || link != "/foreign/missing" {
		t.Fatal("foreign link changed")
	}
}
