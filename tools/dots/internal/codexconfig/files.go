package codexconfig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func readText(path string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("%s: invalid UTF-8", path)
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n"), nil
}
func readExisting(path string) (string, bool, error) {
	_, e := os.Stat(path)
	if errors.Is(e, os.ErrNotExist) {
		return "", false, nil
	}
	if e != nil {
		return "", false, e
	}
	s, e := readText(path)
	return s, true, e
}
func unchanged(path string, existed bool, text string) error {
	s, exists, e := readExisting(path)
	if e != nil {
		return e
	}
	if exists != existed || (exists && s != text) {
		return fmt.Errorf("%s changed while Codex config was being prepared", path)
	}
	return nil
}
func resolve(path string) (string, error) {
	// Resolve each symlink before processing a following "..", including
	// nonexistent targets. Cleaning the entire path first changes its meaning.
	if !filepath.IsAbs(path) {
		cwd, e := os.Getwd()
		if e != nil {
			return "", e
		}
		path = cwd + string(os.PathSeparator) + path
	}
	pending := strings.Split(path, string(os.PathSeparator))
	current := string(os.PathSeparator)
	links := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			current = filepath.Dir(current)
			continue
		}
		next := filepath.Join(current, part)
		info, e := os.Lstat(next)
		if errors.Is(e, os.ErrNotExist) {
			current = next
			continue
		}
		if e != nil {
			return "", e
		}
		if info.Mode()&os.ModeSymlink == 0 {
			current = next
			continue
		}
		links++
		if links > 255 {
			return "", fmt.Errorf("too many symbolic links resolving config path")
		}
		link, e := os.Readlink(next)
		if e != nil {
			return "", e
		}
		if filepath.IsAbs(link) {
			current = string(os.PathSeparator)
		}
		pending = append(strings.Split(link, string(os.PathSeparator)), pending...)
	}
	return current, nil
}
func distinct(a, b, description string) error {
	x, e := resolve(a)
	if e != nil {
		return e
	}
	y, e := resolve(b)
	if e != nil {
		return e
	}
	if x == y {
		return fmt.Errorf("Codex config %s must be different files", description)
	}
	return nil
}
func atomicWrite(path, text string, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0777); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = f.WriteString(text); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Chmod(mode); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func backupFile(target, text string) (string, error) {
	base := target + ".bak-" + time.Now().Format("20060102150405")
	for i := 0; i < 10000; i++ {
		p := base
		if i > 0 {
			p = fmt.Sprintf("%s.%d", base, i)
		}
		// Exclusive creation also protects existing dangling backup symlinks.
		f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(e, os.ErrExist) {
			continue
		}
		if e != nil {
			return "", e
		}
		_, e = f.WriteString(text)
		if e == nil {
			e = f.Sync()
		}
		if e == nil {
			e = f.Chmod(0600)
		}
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			_ = os.Remove(p)
			return "", e
		}
		return p, nil
	}
	return "", fmt.Errorf("unable to allocate a Codex config backup")
}
func validateCodex(text, binary string) error {
	dir, e := os.MkdirTemp("", "codex-config-validate-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	if e = os.WriteFile(filepath.Join(dir, "config.toml"), []byte(text), 0600); e != nil {
		return e
	}
	if e = os.Chmod(filepath.Join(dir, "config.toml"), 0600); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--strict-config", "app-server")
	cmd.Env = append(os.Environ(), "CODEX_HOME="+dir)
	cmd.WaitDelay = time.Second
	e = cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("Codex strict config validation timed out")
	}
	if e != nil {
		return fmt.Errorf("Codex strict config validation failed")
	}
	return nil
}
