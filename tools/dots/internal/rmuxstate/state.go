package rmuxstate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/sys/unix"
)

type Snapshot struct {
	Version  int       `json:"version"`
	Sessions []Session `json:"sessions"`
}
type Session struct {
	Name    string   `json:"name"`
	Windows []Window `json:"windows"`
}
type Window struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Layout string `json:"layout"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Active bool   `json:"active"`
	Panes  []Pane `json:"panes"`
}
type Pane struct {
	ID           string `json:"id"`
	Index        int    `json:"index"`
	CWD          string `json:"cwd"`
	Command      string `json:"command,omitempty"`
	StartCommand string `json:"start_command,omitempty"`
	Active       bool   `json:"active"`
	Jcode        string `json:"jcode,omitempty"`
}

var sessionID = regexp.MustCompile(`^session_[A-Za-z0-9_]+$`)
var paneID = regexp.MustCompile(`^%[0-9]+$`)
var snapshotName = regexp.MustCompile(`^snapshot-[0-9a-f]{64}\.json$`)

const maxBytes = 8 << 20

func cleanText(s string, max int) bool {
	return len(s) <= max && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) })
}
func safeName(s string) bool {
	return len(s) > 0 && cleanText(s, 128) && !strings.HasPrefix(s, "-") && !strings.ContainsAny(s, ":.\"'\\;$%#{}[]")
}
func (s Snapshot) Validate() error {
	if s.Version != 1 || len(s.Sessions) == 0 || len(s.Sessions) > 100 {
		return errors.New("invalid or empty snapshot")
	}
	sessions := map[string]bool{}
	ids := map[string]bool{}
	total := 0
	for _, se := range s.Sessions {
		if !safeName(se.Name) || sessions[se.Name] || len(se.Windows) == 0 || len(se.Windows) > 100 {
			return errors.New("invalid or duplicate session")
		}
		sessions[se.Name] = true
		wi := map[int]bool{}
		activeWindows := 0
		for _, w := range se.Windows {
			if w.Index < 0 || w.Index > 10000 || wi[w.Index] || !cleanText(w.Name, 512) || w.Width < 1 || w.Width > 10000 || w.Height < 1 || w.Height > 10000 || len(w.Panes) == 0 || len(w.Panes) > 128 {
				return errors.New("invalid window")
			}
			wi[w.Index] = true
			if w.Active {
				activeWindows++
			}
			leaves, err := layoutIDs(w.Layout)
			if err != nil || len(leaves) != len(w.Panes) {
				return errors.New("invalid layout")
			}
			leafSet := map[string]bool{}
			for _, id := range leaves {
				if leafSet[id] {
					return errors.New("duplicate layout leaf")
				}
				leafSet[id] = true
			}
			pi := map[int]bool{}
			activePanes := 0
			for _, p := range w.Panes {
				total++
				if total > 1000 || !paneID.MatchString(p.ID) || ids[p.ID] || !leafSet[p.ID] || p.Index < 0 || p.Index > 10000 || pi[p.Index] || !filepath.IsAbs(p.CWD) || !cleanText(p.CWD, 4096) || !cleanText(p.Command, 4096) || !cleanText(p.StartCommand, 16384) || (p.Jcode != "" && (!sessionID.MatchString(p.Jcode) || len(p.Jcode) > 256)) {
					return errors.New("invalid pane")
				}
				ids[p.ID] = true
				pi[p.Index] = true
				if p.Active {
					activePanes++
				}
			}
			if activePanes != 1 {
				return errors.New("invalid active pane")
			}
		}
		if activeWindows != 1 {
			return errors.New("invalid active window")
		}
	}
	return nil
}

type Store struct {
	Dir  string
	dir  *os.File
	lock *os.File
}

func secureDir(path string, private bool) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("directory must be absolute and clean")
	}
	// macOS exposes these root-owned system aliases even for isolated temporary homes.
	for _, alias := range []string{"/var/", "/tmp/"} {
		if strings.HasPrefix(path, alias) {
			real, e := filepath.EvalSymlinks(strings.TrimSuffix(alias, "/"))
			if e != nil {
				return nil, e
			}
			path = real + path[len(alias)-1:]
			break
		}
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if e != nil {
		return nil, e
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for n, part := range parts {
		if part == "" {
			continue
		}
		if private {
			var git unix.Stat_t
			if err := unix.Fstatat(fd, ".git", &git, unix.AT_SYMLINK_NOFOLLOW); err == nil || !errors.Is(err, unix.ENOENT) {
				unix.Close(fd)
				return nil, errors.New("state directory must remain outside Git")
			}
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) {
			if err = unix.Mkdirat(fd, part, 0700); err == nil || errors.Is(err, unix.EEXIST) {
				next, err = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		unix.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
		if private && n == len(parts)-1 {
			var git unix.Stat_t
			if err := unix.Fstatat(fd, ".git", &git, unix.AT_SYMLINK_NOFOLLOW); err == nil || !errors.Is(err, unix.ENOENT) {
				unix.Close(fd)
				return nil, errors.New("state directory must remain outside Git")
			}
		}
		var st unix.Stat_t
		if err = unix.Fstat(fd, &st); err != nil {
			unix.Close(fd)
			return nil, err
		}
		own := st.Uid == uint32(os.Getuid())
		root := st.Uid == 0
		unsafe := st.Mode&0022 != 0 && !(root && st.Mode&unix.S_ISVTX != 0)
		if (!own && !root) || unsafe || (n == len(parts)-1 && private && (!own || st.Mode&0777 != 0700)) {
			unix.Close(fd)
			return nil, fmt.Errorf("unsafe directory %s", path)
		}
	}
	return os.NewFile(uintptr(fd), path), nil
}
func Open(dir string) (*Store, error) {
	d, e := secureDir(dir, true)
	if e != nil {
		return nil, e
	}
	s := &Store{Dir: dir, dir: d}
	f, e := s.openFile("lock", unix.O_RDWR|unix.O_CREAT)
	if e != nil {
		d.Close()
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX); e != nil {
		f.Close()
		d.Close()
		return nil, e
	}
	s.lock = f
	return s, nil
}
func (s *Store) Close() {
	if s.lock != nil {
		unix.Flock(int(s.lock.Fd()), unix.LOCK_UN)
		s.lock.Close()
	}
	if s.dir != nil {
		s.dir.Close()
	}
}
func (s *Store) openFile(name string, flags int) (*os.File, error) {
	if filepath.Base(name) != name {
		return nil, errors.New("unsafe filename")
	}
	fd, e := unix.Openat(int(s.dir.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), name)
	var st unix.Stat_t
	e = unix.Fstat(fd, &st)
	if e != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600 || st.Uid != uint32(os.Getuid()) || st.Nlink != 1 {
		f.Close()
		return nil, fmt.Errorf("unsafe private file %s", name)
	}
	return f, nil
}
func (s *Store) read(name string) ([]byte, error) {
	f, e := s.openFile(name, unix.O_RDONLY)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if len(b) > maxBytes {
		return nil, errors.New("state too large")
	}
	return b, e
}
func (s *Store) atomic(name string, b []byte) error {
	if f, e := s.openFile(name, unix.O_RDONLY); e == nil {
		f.Close()
	} else if !errors.Is(e, unix.ENOENT) {
		return e
	}
	// The temporary is created in the already-open private directory, not through a mutable path.
	var tmp string
	var f *os.File
	for n := 0; n < 100; n++ {
		tmp = fmt.Sprintf(".tmp-%d-%d", os.Getpid(), n)
		var e error
		f, e = s.openFile(tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL)
		if e == nil {
			break
		}
		if !errors.Is(e, unix.EEXIST) {
			return e
		}
	}
	if f == nil {
		return errors.New("temporary collision")
	}
	defer unix.Unlinkat(int(s.dir.Fd()), tmp, 0)
	_, e := f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = unix.Renameat(int(s.dir.Fd()), tmp, int(s.dir.Fd()), name); e != nil {
		return e
	}
	return s.dir.Sync()
}
func decodeSnapshot(b []byte) (Snapshot, error) {
	var v Snapshot
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&v); e != nil {
		return v, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return v, errors.New("trailing snapshot data")
	}
	return v, v.Validate()
}
func (s *Store) Load() (Snapshot, error) {
	b, e := s.read("latest")
	if e != nil {
		return Snapshot{}, e
	}
	name := string(b)
	if !snapshotName.MatchString(name) {
		return Snapshot{}, errors.New("invalid latest pointer")
	}
	b, e = s.read(name)
	if e != nil {
		return Snapshot{}, e
	}
	sum := sha256.Sum256(b)
	if name != "snapshot-"+hex.EncodeToString(sum[:])+".json" {
		return Snapshot{}, errors.New("snapshot checksum mismatch")
	}
	return decodeSnapshot(b)
}
func (s *Store) Save(v Snapshot) (bool, error) {
	if e := v.Validate(); e != nil {
		return false, e
	}
	old, e := s.Load()
	if e == nil {
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(v)
		if bytes.Equal(a, b) {
			return false, nil
		}
	} else if !errors.Is(e, unix.ENOENT) {
		return false, e
	} else {
		if _, pe := s.read("latest"); pe == nil {
			return false, e
		}
	}
	b, e := json.Marshal(v)
	if e != nil {
		return false, e
	}
	if len(b) > maxBytes {
		return false, errors.New("snapshot exceeds state size limit")
	}
	sum := sha256.Sum256(b)
	name := "snapshot-" + hex.EncodeToString(sum[:]) + ".json"
	if e = s.atomic(name, b); e != nil {
		return false, e
	}
	if e = s.atomic("latest", []byte(name)); e != nil {
		return false, e
	}
	s.prune(name)
	return true, nil
}
func (s *Store) prune(latest string) {
	entries, e := s.dir.ReadDir(-1)
	if e != nil {
		return
	}
	type entry struct {
		name string
		time int64
	}
	var candidates []entry
	for _, de := range entries {
		if !snapshotName.MatchString(de.Name()) || de.Name() == latest {
			continue
		}
		b, e := s.read(de.Name())
		if e != nil {
			continue
		}
		if _, e = decodeSnapshot(b); e != nil {
			continue
		}
		sum := sha256.Sum256(b)
		if de.Name() != "snapshot-"+hex.EncodeToString(sum[:])+".json" {
			continue
		}
		i, e := de.Info()
		if e == nil {
			candidates = append(candidates, entry{de.Name(), i.ModTime().UnixNano()})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].time > candidates[j].time })
	if len(candidates) > 60 {
		for _, de := range candidates[60:] {
			unix.Unlinkat(int(s.dir.Fd()), de.name, 0)
		}
	}
	s.dir.Seek(0, 0)
}
