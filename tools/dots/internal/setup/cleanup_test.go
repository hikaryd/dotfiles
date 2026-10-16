package setup

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fixtureCleanup(t *testing.T) (string, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, root := filepath.Join(base, "home"), filepath.Join(base, "repo")
	if err := os.MkdirAll(h, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", h)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, "cache"))
	put(t, filepath.Join(root, "steps/terminal.yml"), "- link:\n    ~/.config/current:\n      path: config/not-created-yet\n")
	put(t, filepath.Join(root, "scripts/spotifast-launch.sh"), "current launcher")
	return root, h
}

func cleanupLink(t *testing.T, path, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func cleanupPaths(plan []cleanupItem) map[string]bool {
	paths := map[string]bool{}
	for _, item := range plan {
		paths[item.path] = true
	}
	return paths
}

func TestCleanupPreviewApplyAndIdempotence(t *testing.T) {
	root, h := fixtureCleanup(t)
	paths := []string{
		filepath.Join(root, "default.profraw"),
		filepath.Join(root, "config/.DS_Store"),
		filepath.Join(root, "scripts/__pycache__/old.cpython-314.pyc"),
		filepath.Join(root, "scripts/tests/__pycache__/test_old.cpython-314.pyc"),
		filepath.Join(h, ".config/xray-pi/__pycache__/codex-proxy.cpython-314.pyc"),
		filepath.Join(h, "cache/dots/spotify-toggle"),
		filepath.Join(h, "cache/dots/spotifast-toggle"),
	}
	for _, p := range paths {
		put(t, p, "obsolete generated artifact")
	}
	link := filepath.Join(h, ".local/bin/spotify-app")
	cleanupLink(t, link, filepath.Join(root, "scripts/spotify-launch.sh"))
	paths = append(paths, link)
	for _, rel := range []string{"config/xray-codex", "extras"} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	keep := []string{
		filepath.Join(root, "scripts/spotifast-launch.sh"),
		filepath.Join(root, "scripts/active.py"),
		filepath.Join(root, "scripts/__pycache__/active.cpython-314.pyc"),
		filepath.Join(root, "config/pi/auth.json"),
		filepath.Join(root, "build/release.tar.gz"),
		filepath.Join(h, ".config/xray-pi/servers.json"),
		filepath.Join(h, ".config/xray-pi/config.json"),
		filepath.Join(h, ".config/xray-pi/selection.json"),
		filepath.Join(h, ".config/xray-pi/subscription_url.txt"),
		filepath.Join(h, "cache/dots/atuin-init.zsh"),
		filepath.Join(h, ".config/dots-backups/old.pyc"),
	}
	for _, p := range keep {
		put(t, p, "keep")
	}
	if code := Run(root, []string{"cleanup"}); code != 0 {
		t.Fatal("preview failed", code)
	}
	for _, p := range paths {
		if _, err := os.Lstat(p); err != nil {
			t.Fatal("preview changed filesystem", p, err)
		}
	}
	if code := Run(root, []string{"cleanup", "--apply"}); code != 0 {
		t.Fatal("apply failed", code)
	}
	for _, p := range paths {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatal("obsolete artifact retained", p, err)
		}
	}
	for _, p := range keep {
		if get(t, p) != "keep" {
			t.Fatal("active/private data changed", p)
		}
	}
	plan, err := cleanupPlan(root, time.Now())
	if err != nil || len(plan) != 0 {
		t.Fatalf("not idempotent: %+v %v", plan, err)
	}
}

func TestCleanupToggleCachesRequireRetiredSources(t *testing.T) {
	for _, name := range []string{"spotify-toggle", "spotifast-toggle"} {
		t.Run(name, func(t *testing.T) {
			root, h := fixtureCleanup(t)
			source := filepath.Join(root, "scripts", name+".sh")
			cache := filepath.Join(h, "cache/dots", name)
			put(t, source, "active source")
			put(t, cache, "compiled toggle")
			plan, err := cleanupPlan(root, time.Now())
			if err != nil || cleanupPaths(plan)[cache] {
				t.Fatal("active toggle cache was not preserved", plan, err)
			}
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			}
			plan, err = cleanupPlan(root, time.Now())
			if err != nil || !cleanupPaths(plan)[cache] {
				t.Fatal("retired toggle cache was not selected", plan, err)
			}
			put(t, source, "restored source")
			if err := applyCleanup(plan); err == nil {
				t.Fatal("cache of a restored source was removed")
			}
			if get(t, cache) != "compiled toggle" {
				t.Fatal("cache changed after its source was restored")
			}
		})
	}
}

func TestCleanupOnlyRemovesUnmanagedBrokenLinksOwnedByThisCheckout(t *testing.T) {
	root, h := fixtureCleanup(t)
	put(t, filepath.Join(root, "config/active"), "active")
	cases := []struct {
		path, target string
		remove       bool
	}{
		{".config/current", filepath.Join(root, "config/not-created-yet"), false},
		{".config/foreign", filepath.Join(root+"-other", "absent"), false},
		{".config/live", filepath.Join(root, "config/active"), false},
		{".local/bin/building", filepath.Join(root, "build/bin/new-command"), false},
		{".local/bin/retired", filepath.Join(root, "scripts/retired.py"), true},
		{".local/bin/relative-retired", "../../../repo/scripts/retired.py", true},
		{".config/retired-directory", filepath.Join(root, "config/removed-app"), true},
	}
	for _, c := range cases {
		cleanupLink(t, filepath.Join(h, c.path), c.target)
	}
	put(t, filepath.Join(h, ".local/bin/custom-command"), "not a dots link")
	plan, err := cleanupPlan(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	paths := cleanupPaths(plan)
	for _, c := range cases {
		if paths[filepath.Join(h, c.path)] != c.remove {
			t.Errorf("wrong ownership/liveness classification: %+v", c)
		}
	}
	if err := applyCleanup(plan); err != nil {
		t.Fatal(err)
	}
	if get(t, filepath.Join(h, ".local/bin/custom-command")) != "not a dots link" {
		t.Fatal("custom executable changed")
	}
}

func TestCleanupNeverFollowsForeignDirectoriesOrScansDependenciesAndBackups(t *testing.T) {
	root, h := fixtureCleanup(t)
	outside := t.TempDir()
	put(t, filepath.Join(outside, "__pycache__/private.cpython-314.pyc"), "private")
	cleanupLink(t, filepath.Join(root, "config/redirect"), outside)
	cleanupLink(t, filepath.Join(h, ".config/xray-pi"), outside)
	for _, rel := range []string{"dotbot", "node_modules", "build", ".venv", "docs/private", "docs/private-backups", "tools/nested/backups"} {
		put(t, filepath.Join(root, rel, "__pycache__/private.cpython-314.pyc"), "private")
	}
	plan, err := cleanupPlan(root, time.Now())
	if err != nil || len(plan) != 0 {
		t.Fatalf("followed foreign/protected path: %+v %v", plan, err)
	}
	if get(t, filepath.Join(outside, "__pycache__/private.cpython-314.pyc")) != "private" {
		t.Fatal("foreign data changed")
	}
}

func TestCleanupRefusesChangedFilesAndRedirectedParents(t *testing.T) {
	for _, change := range []string{"file", "parent", "restored-source", "nonempty-directory"} {
		t.Run(change, func(t *testing.T) {
			root, _ := fixtureCleanup(t)
			p := filepath.Join(root, "config/.DS_Store")
			if change == "restored-source" {
				p = filepath.Join(root, "scripts/__pycache__/restored.cpython-314.pyc")
			}
			if change == "nonempty-directory" {
				p = filepath.Join(root, "config/xray-codex")
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				put(t, p, "original")
			}
			plan, err := cleanupPlan(root, time.Now())
			if err != nil || len(plan) == 0 {
				t.Fatal(plan, err)
			}
			switch change {
			case "file":
				put(t, p, "changed contents")
			case "parent":
				outside := t.TempDir()
				put(t, filepath.Join(outside, ".DS_Store"), "foreign")
				if err := os.Rename(filepath.Dir(p), filepath.Dir(p)+"-original"); err != nil {
					t.Fatal(err)
				}
				cleanupLink(t, filepath.Dir(p), outside)
			case "restored-source":
				put(t, filepath.Join(root, "scripts/restored.py"), "restored")
			case "nonempty-directory":
				put(t, filepath.Join(p, "new-user-data"), "preserve")
			}
			if err := applyCleanup(plan); err == nil {
				t.Fatal("changed candidate was not rejected")
			}
			if _, err := os.Lstat(p); err != nil {
				t.Fatal("changed/redirected path deleted", err)
			}
		})
	}
}

func TestCleanupTemporaryFilesRequireStaleAgeAndDeadOwner(t *testing.T) {
	root, h := fixtureCleanup(t)
	now := time.Now()
	files := []struct {
		name string
		old  bool
		want bool
	}{
		{"atuin-init.zsh.1073741823.tmp", true, true},
		{"atuin-init.zsh.1073741822.tmp", false, false},
		{"atuin-init.zsh." + strconv.Itoa(os.Getpid()) + ".tmp", true, false},
		{"atuin-init.zsh.unknown.tmp", true, false},
	}
	for _, f := range files {
		p := filepath.Join(h, "cache/dots", f.name)
		put(t, p, "temp")
		if f.old {
			old := now.Add(-48 * time.Hour)
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	plan, err := cleanupPlan(root, now)
	if err != nil {
		t.Fatal(err)
	}
	paths := cleanupPaths(plan)
	for _, f := range files {
		if paths[filepath.Join(h, "cache/dots", f.name)] != f.want {
			t.Error("temporary file misclassified", f)
		}
	}
}

func TestCleanupRejectsInvalidArgsOrMissingInventoryWithoutDeleting(t *testing.T) {
	root, _ := fixtureCleanup(t)
	put(t, filepath.Join(root, "default.profraw"), "keep on failure")
	for _, args := range [][]string{{"cleanup", "--unknown"}, {"cleanup", "--apply", "--extra"}} {
		if Run(root, args) == 0 {
			t.Fatal("invalid arguments accepted", args)
		}
	}
	put(t, filepath.Join(root, "steps/terminal.yml"), "not: [valid")
	if err := cleanup(root, []string{"--apply"}); err == nil || !strings.Contains(err.Error(), "inventory") {
		t.Fatal("invalid manifest accepted", err)
	}
	if get(t, filepath.Join(root, "default.profraw")) != "keep on failure" {
		t.Fatal("artifact removed without a reliable installation inventory")
	}
}
