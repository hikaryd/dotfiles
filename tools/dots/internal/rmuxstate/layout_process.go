package rmuxstate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func withChecksum(body string) string {
	var sum uint16
	for _, b := range []byte(body) {
		sum = (sum >> 1) | (sum << 15)
		sum += uint16(b)
	}
	return fmt.Sprintf("%04x,%s", sum, body)
}

type layoutParser struct {
	body string
	pos  int
	ids  []string
}

func (p *layoutParser) number() (int, error) {
	start := p.pos
	for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
		p.pos++
	}
	if start == p.pos || p.pos-start > 8 {
		return 0, errors.New("invalid layout number")
	}
	return strconv.Atoi(p.body[start:p.pos])
}
func (p *layoutParser) expect(c byte) error {
	if p.pos >= len(p.body) || p.body[p.pos] != c {
		return errors.New("invalid layout delimiter")
	}
	p.pos++
	return nil
}
func (p *layoutParser) cell(depth int) error {
	if depth > 64 {
		return errors.New("layout too deep")
	}
	for i, sep := range []byte{'x', ',', ',', 0} {
		n, e := p.number()
		if e != nil || n > 100000 || (i < 2 && n == 0) {
			return errors.New("invalid layout geometry")
		}
		if sep != 0 {
			if e = p.expect(sep); e != nil {
				return e
			}
		}
	}
	if p.pos < len(p.body) && (p.body[p.pos] == '{' || p.body[p.pos] == '[') {
		close := byte('}')
		if p.body[p.pos] == '[' {
			close = ']'
		}
		p.pos++
		children := 0
		for {
			if e := p.cell(depth + 1); e != nil {
				return e
			}
			children++
			if children > 128 {
				return errors.New("too many layout children")
			}
			if p.pos >= len(p.body) {
				return errors.New("unclosed layout")
			}
			c := p.body[p.pos]
			p.pos++
			if c == close {
				break
			}
			if c != ',' {
				return errors.New("invalid layout split")
			}
		}
		if children < 2 {
			return errors.New("empty layout split")
		}
		return nil
	}
	if e := p.expect(','); e != nil {
		return e
	}
	id, e := p.number()
	if e != nil {
		return e
	}
	p.ids = append(p.ids, "%"+strconv.Itoa(id))
	if len(p.ids) > 128 {
		return errors.New("too many panes")
	}
	return nil
}
func layoutIDs(layout string) ([]string, error) {
	if len(layout) > 32768 || len(layout) < 6 || layout[4] != ',' || withChecksum(layout[5:]) != layout {
		return nil, errors.New("invalid layout checksum")
	}
	p := layoutParser{body: layout[5:]}
	if e := p.cell(0); e != nil {
		return nil, e
	}
	if p.pos != len(p.body) {
		return nil, errors.New("trailing layout")
	}
	return p.ids, nil
}

func jcodeFile(path string, limit int) (*os.File, error) {
	parent, e := secureExistingDir(filepath.Dir(path))
	if e != nil {
		return nil, e
	}
	defer parent.Close()
	var ds unix.Stat_t
	if e = unix.Fstat(int(parent.Fd()), &ds); e != nil || ds.Uid != uint32(os.Getuid()) || ds.Mode&0777 != 0700 {
		return nil, errors.New("unsafe Jcode directory")
	}
	fd, e := unix.Openat(int(parent.Fd()), filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	var st unix.Stat_t
	if e = unix.Fstat(fd, &st); e != nil || st.Uid != uint32(os.Getuid()) || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0022 != 0 || st.Nlink != 1 || st.Size < 1 || st.Size > int64(limit) {
		f.Close()
		return nil, errors.New("unsafe Jcode metadata")
	}
	return f, nil
}
func privateRead(path string, limit int) ([]byte, error) {
	f, e := jcodeFile(path, limit)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(limit)+1))
}
func secureExistingDir(path string) (*os.File, error) {
	// Never create a missing history directory while inspecting a saved Jcode session.
	if _, e := os.Lstat(path); e != nil {
		return nil, e
	}
	return secureDir(path, false)
}
func validHistory(home, id string) bool {
	if !sessionID.MatchString(id) || len(id) > 256 {
		return false
	}
	f, e := jcodeFile(filepath.Join(home, ".jcode", "sessions", id+".json"), 64<<20)
	if e != nil {
		return false
	}
	f.Close()
	return true
}

type Process struct {
	Parent     int
	Executable string
	Started    time.Time
}

func descendant(root, pid int, processes map[int]Process) bool {
	seen := map[int]bool{}
	for pid > 0 && !seen[pid] {
		if pid == root {
			return true
		}
		seen[pid] = true
		pid = processes[pid].Parent
	}
	return false
}
func hasJcodeDescendant(root int, processes map[int]Process) bool {
	for pid, p := range processes {
		if filepath.Base(p.Executable) == "jcode" && descendant(root, pid, processes) {
			return true
		}
	}
	return false
}
func ResolveJcode(home string, root int, processes map[int]Process) string {
	found := ""
	for pid, p := range processes {
		if filepath.Base(p.Executable) != "jcode" || p.Started.IsZero() || !descendant(root, pid, processes) || !trustedExecutable(p.Executable) {
			continue
		}
		mapping, e := jcodeFile(filepath.Join(home, ".jcode", "client_sessions", strconv.Itoa(pid)), 512)
		if e != nil {
			continue
		}
		info, e := mapping.Stat()
		if e != nil || info.ModTime().Before(p.Started) {
			mapping.Close()
			continue
		}
		b, e := io.ReadAll(io.LimitReader(mapping, 513))
		after, se := mapping.Stat()
		mapping.Close()
		if e != nil || se != nil || !info.ModTime().Equal(after.ModTime()) || info.Size() != after.Size() || len(b) > 512 {
			continue
		}
		id := strings.TrimSpace(string(b))
		if !validHistory(home, id) {
			continue
		}
		if found != "" && found != id {
			return ""
		}
		found = id
	}
	return found
}
