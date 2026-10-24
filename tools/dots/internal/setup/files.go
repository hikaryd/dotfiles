package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

func home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if u, err := user.Current(); err == nil {
		return u.HomeDir
	}
	return ""
}
func expand(s string) string {
	if s == "~" {
		return home()
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home(), s[2:])
	}
	return s
}
func envDefault(names []string, fallback string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return fallback
}
func absolute(p string) string {
	v, e := filepath.Abs(expand(p))
	if e != nil {
		return expand(p)
	}
	return v
}
func resolve(p string) (string, error) {
	p = absolute(p)
	for i := 0; i < 64; i++ {
		q, e := filepath.EvalSymlinks(p)
		if e == nil {
			return q, nil
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		info, e := os.Lstat(p)
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			q, e := os.Readlink(p)
			if e != nil {
				return "", e
			}
			if !filepath.IsAbs(q) {
				q = filepath.Join(filepath.Dir(p), q)
			}
			p = q
			continue
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p, nil
		}
		q, e = resolve(parent)
		if e != nil {
			return "", e
		}
		return filepath.Join(q, filepath.Base(p)), nil
	}
	return "", errors.New("too many symbolic links")
}
func readOptional(p string) ([]byte, error) {
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return nil, nil
	}
	return b, e
}
func object(b []byte) (map[string]any, error) {
	var d map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if e := dec.Decode(&d); e != nil || d == nil {
		return nil, errors.New("expected a valid JSON object")
	}
	var tail any
	if e := dec.Decode(&tail); !errors.Is(e, io.EOF) {
		return nil, errors.New("expected a single JSON object")
	}
	return d, nil
}
func readObject(p string, optional bool) (map[string]any, error) {
	b, e := os.ReadFile(p)
	if optional && os.IsNotExist(e) {
		return map[string]any{}, nil
	}
	if e != nil {
		return nil, fmt.Errorf("cannot read %s", filepath.Base(p))
	}
	d, e := object(b)
	if e != nil {
		return nil, fmt.Errorf("invalid JSON object in %s", filepath.Base(p))
	}
	return d, nil
}
func encode(d any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	_ = e.Encode(d)
	return b.Bytes()
}
func merge(a, b map[string]any) map[string]any {
	r := map[string]any{}
	for k, v := range a {
		r[k] = v
	}
	for k, v := range b {
		vm, ok := v.(map[string]any)
		rm, rok := r[k].(map[string]any)
		if ok && rok {
			r[k] = merge(rm, vm)
		} else {
			r[k] = v
		}
	}
	return r
}
func stamp() string {
	return fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), time.Now().UnixNano())
}
func privateBackup(group, p string) string {
	return filepath.Join(home(), ".config/dots-backups", group, stamp(), filepath.Base(p))
}

type change struct {
	path          string
	data          []byte
	link          string
	backup        string
	mode          os.FileMode
	follow        bool
	backupPrivate bool
	expect        bool
	expected      []byte
}
type staged struct {
	change
	temp, old string
	info      os.FileInfo
	prior     []byte
	priorLink string
	committed bool
	moved     bool
	copied    bool
}

// All inputs and replacements are staged before publication. Each rename is atomic;
// failure rolls back earlier publications, including moved directories and links.
func commit(changes []change) (err error) {
	var stagedFiles []*staged
	seenPaths := map[string]bool{}
	defer func() {
		for _, s := range stagedFiles {
			if s.temp != "" {
				_ = os.Remove(s.temp)
			}
		}
	}()
	for _, c := range changes {
		if c.follow {
			c.path, err = resolve(c.path)
			if err != nil {
				return err
			}
		}
		parent, e := resolve(filepath.Dir(c.path))
		if e != nil {
			return e
		}
		identity := filepath.Join(parent, filepath.Base(c.path))
		if seenPaths[identity] {
			return errors.New("duplicate installation destination")
		}
		seenPaths[identity] = true
		s := &staged{change: c}
		s.info, err = os.Lstat(c.path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		err = nil
		if c.expect && ((c.expected == nil) != (s.info == nil)) {
			return errors.New("configuration changed during installation")
		}
		if s.info != nil {
			if s.info.Mode()&os.ModeSymlink != 0 {
				s.priorLink, err = os.Readlink(c.path)
				if err != nil {
					return err
				}
				if c.link == "" {
					return errors.New("expected a regular file")
				}
				if q, e := resolve(c.path); e == nil {
					r, e := resolve(c.link)
					if e == nil && q == r {
						continue
					}
				}
			} else if s.info.Mode().IsRegular() {
				s.prior, err = os.ReadFile(c.path)
				if err != nil {
					return err
				}
				if c.expect && !bytes.Equal(s.prior, c.expected) {
					return errors.New("configuration changed during installation")
				}
				if c.link == "" && bytes.Equal(s.prior, c.data) && (c.mode == 0 || c.mode == s.info.Mode().Perm()) {
					continue
				}
			} else if c.link == "" {
				return errors.New("expected a regular file")
			}
		}
		stagedFiles = append(stagedFiles, s)
	}
	// Complete type/source validation before creating any staging files.
	for _, s := range stagedFiles {
		c := s.change
		if err = os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
			return err
		}
		if c.backup != "" && s.info != nil {
			if err = os.MkdirAll(filepath.Dir(c.backup), 0700); err != nil {
				return err
			}
		}
		f, e := os.CreateTemp(filepath.Dir(c.path), "."+filepath.Base(c.path)+".dots-")
		if e != nil {
			return e
		}
		s.temp = f.Name()
		if c.link != "" {
			if e = f.Close(); e != nil {
				return e
			}
			if e = os.Remove(s.temp); e != nil {
				return e
			}
			if e = os.Symlink(c.link, s.temp); e != nil {
				return e
			}
		} else {
			mode := c.mode
			if mode == 0 {
				mode = 0600
				if s.info != nil {
					mode = s.info.Mode().Perm()
				}
			}
			if e = f.Chmod(mode); e == nil {
				_, e = f.Write(c.data)
			}
			if e == nil {
				e = f.Sync()
			}
			ce := f.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
		}
	}
	// Verify every original before mutating the first one.
	for _, s := range stagedFiles {
		i, e := os.Lstat(s.path)
		if s.info == nil {
			if !os.IsNotExist(e) {
				return errors.New("configuration changed during installation")
			}
			continue
		}
		if e != nil || !os.SameFile(i, s.info) {
			return errors.New("configuration changed during installation")
		}
		if s.info.Mode().IsRegular() {
			b, e := os.ReadFile(s.path)
			if e != nil || !bytes.Equal(b, s.prior) {
				return errors.New("configuration changed during installation")
			}
		}
		if s.priorLink != "" {
			v, e := os.Readlink(s.path)
			if e != nil || v != s.priorLink {
				return errors.New("configuration changed during installation")
			}
		}
	}
	finalized := false
	defer func() {
		if err != nil && !finalized {
			for i := len(stagedFiles) - 1; i >= 0; i-- {
				s := stagedFiles[i]
				if s.committed && !s.copied {
					_ = os.RemoveAll(s.path)
				}
				if s.moved {
					if s.copied && !s.committed {
						_ = os.Remove(s.old)
						continue
					}
					if e := os.Rename(s.old, s.path); e != nil {
						err = fmt.Errorf("installation failed; rollback failed for %s", filepath.Base(s.path))
					} else if s.info.Mode().IsRegular() {
						if e := os.Chmod(s.path, s.info.Mode().Perm()); e != nil {
							err = errors.New("installation failed; could not restore file permissions")
						}
					}
				}
			}
		}
	}()
	for _, s := range stagedFiles {
		if s.info != nil {
			s.old = s.backup
			if s.old == "" {
				s.old = s.temp + ".rollback"
			}
			if _, e := os.Lstat(s.old); e == nil {
				return errors.New("backup destination already exists")
			}
			if s.info.Mode().IsRegular() {
				// Copy the rollback image, then atomically replace the live path.
				// Moving the live file first would expose a missing-file window.
				f, e := os.OpenFile(s.old, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if e != nil {
					return e
				}
				s.moved = true
				s.copied = true
				_, e = f.Write(s.prior)
				if e == nil {
					e = f.Sync()
				}
				ce := f.Close()
				if e != nil {
					return e
				}
				if ce != nil {
					return ce
				}
				mode := s.info.Mode().Perm()
				if s.backupPrivate {
					mode = 0600
				}
				if err = os.Chmod(s.old, mode); err != nil {
					return err
				}
				if err = os.Chtimes(s.old, s.info.ModTime(), s.info.ModTime()); err != nil {
					return err
				}
			} else {
				if err = os.Rename(s.path, s.old); err != nil {
					return err
				}
				s.moved = true
			}
		}
		if err = os.Rename(s.temp, s.path); err != nil {
			return err
		}
		s.temp = ""
		s.committed = true
	}
	// Publication is complete. A cleanup failure must never delete newly
	// installed files after an earlier rollback image has already been removed.
	finalized = true
	for _, s := range stagedFiles {
		if s.moved && s.backup == "" {
			if err = os.RemoveAll(s.old); err != nil {
				return err
			}
			s.moved = false
		}
	}
	return nil
}
