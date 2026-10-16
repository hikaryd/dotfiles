package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type cleanupItem struct {
	path, reason string
	boundary     string
	info         os.FileInfo
	eligible     func() bool
}

func within(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// WalkDir does not follow leaf symlinks, but its starting path/ancestors might.
// Refuse redirected managed directories as well as redirected parents at apply.
func cleanupParents(path, boundary string) bool {
	if !within(path, boundary) {
		return false
	}
	for p := filepath.Dir(path); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if p == boundary {
			return true
		}
		if p == filepath.Dir(p) {
			return false
		}
	}
}

func cleanupSkipDir(name string) bool {
	switch name {
	case ".git", "dotbot", "node_modules", "build", "target", ".venv", ".dual-graph", ".mypy_cache", ".ruff_cache", ".pytest_cache", ".playwright-mcp", "private", "sessions", "projects", "logs":
		return true
	}
	return strings.Contains(strings.ToLower(name), "backup") || strings.HasSuffix(name, ".bak")
}

func orphanBytecode(path string) bool {
	if filepath.Base(filepath.Dir(path)) != "__pycache__" || !strings.HasSuffix(path, ".pyc") {
		return false
	}
	parent := filepath.Dir(filepath.Dir(path))
	stem, _, standard := strings.Cut(filepath.Base(path), ".cpython-")
	if standard {
		_, err := os.Stat(filepath.Join(parent, stem+".py"))
		return errors.Is(err, os.ErrNotExist)
	}
	// Older custom compileall invocations also left non-PEP-3147 filenames.
	// Only remove those when the whole source directory has no Python sources.
	sources, err := filepath.Glob(filepath.Join(parent, "*.py"))
	return err == nil && len(sources) == 0
}

func cleanupPlan(root string, now time.Time) ([]cleanupItem, error) {
	root, err := filepath.EvalSymlinks(absolute(root))
	if err != nil {
		return nil, err
	}
	h, err := filepath.EvalSymlinks(absolute(home()))
	if err != nil {
		return nil, err
	}
	links, err := managedLinkTargets(root)
	if err != nil || len(links) == 0 {
		return nil, errors.New("cleanup requires a valid, nonempty steps/*.yml link inventory")
	}
	managed := map[string]bool{}
	for _, link := range links {
		path := absolute(link)
		parent, err := resolve(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		managed[filepath.Join(parent, filepath.Base(path))] = true
	}
	items := map[string]cleanupItem{}
	add := func(path, boundary, reason string, eligible func() bool) {
		info, err := os.Lstat(path)
		if err == nil && cleanupParents(path, boundary) && eligible() {
			items[path] = cleanupItem{path, reason, boundary, info, eligible}
		}
	}
	var cacheDirs []string
	walk := func(base, boundary string, depth int, repo, bytecode bool) error {
		if base != boundary && !cleanupParents(base, boundary) {
			return nil
		}
		return filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if !repo && !managed[path] {
					add(path, boundary, "broken link to a retired dots source", func() bool {
						target, err := os.Readlink(path)
						if err != nil {
							return false
						}
						if !filepath.IsAbs(target) {
							target = filepath.Join(filepath.Dir(path), target)
						}
						target, err = resolve(target)
						if err != nil {
							return false
						}
						_, err = os.Stat(path)
						return within(target, root) && !within(target, filepath.Join(root, "build")) && errors.Is(err, os.ErrNotExist)
					})
				}
				return nil
			}
			if entry.IsDir() {
				if path != base {
					rel, _ := filepath.Rel(base, path)
					if cleanupSkipDir(entry.Name()) || strings.Count(rel, string(os.PathSeparator)) >= depth {
						return filepath.SkipDir
					}
				}
				if bytecode && entry.Name() == "__pycache__" {
					cacheDirs = append(cacheDirs, path)
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			if repo && (entry.Name() == ".DS_Store" || path == filepath.Join(root, "default.profraw")) {
				add(path, boundary, "generated repository artifact", func() bool { return true })
			}
			if bytecode && orphanBytecode(path) {
				add(path, boundary, "bytecode of a removed Python implementation", func() bool { return orphanBytecode(path) })
			}
			return nil
		})
	}
	for _, scope := range []struct {
		base, boundary string
		depth          int
		repo, bytecode bool
	}{
		{root, root, 64, true, true},
		{h, h, 0, false, false},
		{filepath.Join(h, ".local/bin"), h, 0, false, false},
		{filepath.Join(h, ".config"), h, 3, false, false},
		{filepath.Join(h, ".agents/skills"), h, 1, false, false},
		{filepath.Join(h, ".pi/agent/extensions"), h, 1, false, false},
		{filepath.Join(h, ".config/xray-pi"), h, 2, false, true},
	} {
		if err := walk(scope.base, scope.boundary, scope.depth, scope.repo, scope.bytecode); err != nil {
			return nil, err
		}
	}
	cache := absolute(envDefault([]string{"XDG_CACHE_HOME"}, "~/.cache"))
	for _, name := range []string{"spotify-toggle", "spotifast-toggle"} {
		oldToggle := filepath.Join(cache, "dots", name)
		add(oldToggle, cache, "compiled cache of a retired music-player toggle", func() bool {
			_, sourceErr := os.Stat(filepath.Join(root, "scripts", name+".sh"))
			_, launcherErr := os.Stat(filepath.Join(root, "scripts/spotifast-launch.sh"))
			info, err := os.Lstat(oldToggle)
			return errors.Is(sourceErr, os.ErrNotExist) && launcherErr == nil && err == nil && info.Mode().IsRegular()
		})
	}
	temps, err := filepath.Glob(filepath.Join(cache, "dots/atuin-init.zsh.*.tmp"))
	if err != nil {
		return nil, err
	}
	for _, path := range temps {
		add(path, cache, "abandoned Atuin initialization temporary file", func() bool {
			info, err := os.Lstat(path)
			pidText := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "atuin-init.zsh."), ".tmp")
			pid, parseErr := strconv.Atoi(pidText)
			return err == nil && info.Mode().IsRegular() && now.Sub(info.ModTime()) > 24*time.Hour &&
				parseErr == nil && pid > 1 && errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
		})
	}
	for _, rel := range []string{"config/codexbar", "config/xray-codex", "config/metalterm", "config/gitui", "config/fzf", "config/look", "extras", "scripts/tests", "vpn/tests"} {
		cacheDirs = append(cacheDirs, filepath.Join(root, rel))
	}
	cacheDirs = append(cacheDirs, filepath.Join(h, ".config/skhd"), filepath.Join(h, ".config/Support/nushell"), filepath.Join(h, ".config/Support"))
	sort.Slice(cacheDirs, func(i, j int) bool { return len(cacheDirs[i]) > len(cacheDirs[j]) })
	for _, path := range cacheDirs {
		entries, err := os.ReadDir(path)
		if err != nil {
			continue
		}
		projectedEmpty := true
		for _, entry := range entries {
			if _, removed := items[filepath.Join(path, entry.Name())]; !removed {
				projectedEmpty = false
			}
		}
		if projectedEmpty {
			boundary := h
			if within(path, root) {
				boundary = root
			}
			// Removal uses rmdir semantics, never recursive deletion.
			add(path, boundary, "empty retired/cache directory", func() bool {
				info, err := os.Lstat(path)
				return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
			})
		}
	}
	plan := make([]cleanupItem, 0, len(items))
	for _, item := range items {
		plan = append(plan, item)
	}
	sort.Slice(plan, func(i, j int) bool {
		left, right := strings.Count(plan[i].path, string(os.PathSeparator)), strings.Count(plan[j].path, string(os.PathSeparator))
		if left != right {
			return left > right
		}
		return plan[i].path < plan[j].path
	})
	return plan, nil
}

func applyCleanup(plan []cleanupItem) error {
	for _, item := range plan {
		info, err := os.Lstat(item.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !cleanupParents(item.path, item.boundary) || !os.SameFile(info, item.info) || !item.eligible() ||
			(!info.IsDir() && (info.Size() != item.info.Size() || !info.ModTime().Equal(item.info.ModTime()))) {
			return fmt.Errorf("cleanup candidate changed; preserved: %q", item.path)
		}
		if err := os.Remove(item.path); err != nil {
			return fmt.Errorf("could not remove cleanup candidate %q: %w", item.path, err)
		}
		fmt.Printf("REMOVED %q (%s)\n", item.path, item.reason)
	}
	return nil
}

func cleanup(root string, args []string) error {
	apply := len(args) == 1 && args[0] == "--apply"
	if len(args) > 1 || len(args) == 1 && !apply && args[0] != "--dry-run" {
		return errors.New("usage: dots-tools setup cleanup [--dry-run|--apply]")
	}
	plan, err := cleanupPlan(root, time.Now())
	if err != nil {
		return err
	}
	if apply {
		return applyCleanup(plan)
	}
	for _, item := range plan {
		fmt.Printf("DRY-RUN %q (%s)\n", item.path, item.reason)
	}
	fmt.Printf("%d confirmed obsolete artifacts; nothing changed. Use --apply to remove.\n", len(plan))
	return nil
}
