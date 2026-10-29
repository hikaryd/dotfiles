package rmuxstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var ErrNoServer = errors.New("no rmux server")

const readyOption = "@dots-rmux-state-ready"

type Runtime struct {
	Home      string
	Bin       string
	Socket    string
	Config    string
	Store     *Store
	Warnings  io.Writer
	run       func(...string) (string, error)
	processes func() (map[int]Process, error)
}

func (r *Runtime) token() string {
	sum := sha256.Sum256([]byte(r.Store.Dir))
	return fmt.Sprintf("v1-%x", sum[:16])
}
func cleanEnv() []string {
	var env []string
	for _, s := range os.Environ() {
		key, _, _ := strings.Cut(s, "=")
		if strings.HasPrefix(key, "RMUX") || strings.HasPrefix(key, "TMUX") || strings.HasPrefix(key, "HERDR_") || key == "DOTS_SOURCE_PANE" || key == "DOTS_MUX_CLIENT" {
			continue
		}
		env = append(env, s)
	}
	return env
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxBytes {
		return 0, errors.New("command output too large")
	}
	return b.Buffer.Write(p)
}
func (r *Runtime) call(args ...string) (string, error) {
	if r.run != nil {
		return r.run(args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	prefix := []string{}
	if r.Socket != "" {
		prefix = append(prefix, "-S", r.Socket)
	}
	prefix = append(prefix, args...)
	cmd := exec.CommandContext(ctx, r.Bin, prefix...)
	cmd.Env = cleanEnv()
	var out, diagnostic boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	if e := cmd.Run(); e != nil {
		message := strings.TrimSpace(diagnostic.String())
		if len(args) > 0 && args[0] == "list-sessions" && strings.HasPrefix(message, "error connecting to ") && (strings.HasSuffix(message, "(No such file or directory)") || strings.HasSuffix(message, "(Connection refused)")) {
			return "", ErrNoServer
		}
		return "", fmt.Errorf("rmux %s failed: %w: %s", args[0], e, message)
	}
	return out.String(), nil
}
func (r *Runtime) ready() bool {
	v, e := r.call("show-options", "-gqv", readyOption)
	return e == nil && strings.TrimSpace(v) == r.token()
}
func (r *Runtime) markReady() error {
	_, e := r.call("set-option", "-g", readyOption, r.token())
	return e
}

type sessionRow struct {
	Name string `json:"session_name"`
}
type windowRow struct {
	Session string `json:"session_name"`
	Index   int    `json:"window_index"`
	Name    string `json:"window_name"`
	Layout  string `json:"window_layout"`
	Width   int    `json:"window_width"`
	Height  int    `json:"window_height"`
	Active  bool   `json:"window_active"`
}
type paneRow struct {
	Session string `json:"session_name"`
	Window  int    `json:"window_index"`
	Index   int    `json:"pane_index"`
	ID      string `json:"pane_id"`
	CWD     string `json:"pane_current_path"`
	Command string `json:"pane_current_command"`
	Active  bool   `json:"pane_active"`
	Dead    bool   `json:"pane_dead"`
}

func readRows[T any](r *Runtime, args ...string) ([]T, string, error) {
	out, e := r.call(args...)
	if e != nil {
		return nil, "", e
	}
	var rows []T
	e = json.Unmarshal([]byte(out), &rows)
	if e != nil {
		return nil, "", fmt.Errorf("invalid rmux %s JSON: %w", args[0], e)
	}
	if len(rows) > 10000 {
		return nil, "", errors.New("too many rmux rows")
	}
	return rows, out, nil
}
func sameWindowTopology(a, b string) bool {
	var left, right []map[string]json.RawMessage
	if json.Unmarshal([]byte(a), &left) != nil || json.Unmarshal([]byte(b), &right) != nil || len(left) == 0 || len(left) != len(right) {
		return false
	}
	// OSC window titles animate independently of topology. Keep the first captured name.
	for _, rows := range [][]map[string]json.RawMessage{left, right} {
		for _, row := range rows {
			raw := bytes.TrimSpace(row["window_name"])
			var name string
			if len(raw) < 2 || raw[0] != '"' || json.Unmarshal(raw, &name) != nil || !cleanText(name, 512) {
				return false
			}
			delete(row, "window_name")
		}
	}
	x, err := json.Marshal(left)
	if err != nil {
		return false
	}
	y, err := json.Marshal(right)
	return err == nil && bytes.Equal(x, y)
}

func (r *Runtime) Capture() (Snapshot, error) {
	v := Snapshot{Version: 1}
	roots := map[string]int{}
	mappings := map[string]string{}
	processQuery := r.processes
	if processQuery == nil {
		processQuery = processTable
	}
	processes, e := processQuery()
	if e != nil {
		return v, e
	}
	sessions, rawS, e := readRows[sessionRow](r, "list-sessions", "--json")
	if e != nil {
		return v, e
	}
	if len(sessions) == 0 {
		return v, errors.New("no sessions to save")
	}
	windows, rawW, e := readRows[windowRow](r, "list-windows", "-a", "--json")
	if e != nil {
		return v, e
	}
	panes, rawP, e := readRows[paneRow](r, "list-panes", "-a", "--json")
	if e != nil {
		return v, e
	}
	for _, se := range sessions {
		ss := Session{Name: se.Name}
		for _, w := range windows {
			if w.Session != se.Name {
				continue
			}
			ww := Window{Index: w.Index, Name: w.Name, Layout: w.Layout, Width: w.Width, Height: w.Height, Active: w.Active}
			for _, p := range panes {
				if p.Session != se.Name || p.Window != w.Index {
					continue
				}
				if p.Dead || !paneID.MatchString(p.ID) {
					return v, errors.New("dead or invalid pane")
				}
				pidText, e := r.call("display-message", "-p", "-t", p.ID, "#{pane_pid}")
				if e != nil {
					return v, e
				}
				pid, e := strconv.Atoi(strings.TrimSpace(pidText))
				if e != nil || pid <= 0 {
					return v, errors.New("invalid live pane PID")
				}
				roots[p.ID] = pid
				start, e := r.call("display-message", "-p", "-t", p.ID, "#{pane_start_command}")
				if e != nil {
					return v, e
				}
				start = strings.TrimSuffix(start, "\n")
				jcode := ResolveJcode(r.Home, pid, processes)
				mappings[p.ID] = jcode
				if jcode == "" && hasJcodeDescendant(pid, processes) {
					return v, errors.New("live Jcode client has no safe exact session mapping, keeping last good snapshot")
				}
				ww.Panes = append(ww.Panes, Pane{ID: p.ID, Index: p.Index, CWD: p.CWD, Command: p.Command, StartCommand: start, Active: p.Active, Jcode: jcode})
			}
			sort.Slice(ww.Panes, func(i, j int) bool { return ww.Panes[i].Index < ww.Panes[j].Index })
			ss.Windows = append(ss.Windows, ww)
		}
		sort.Slice(ss.Windows, func(i, j int) bool { return ss.Windows[i].Index < ss.Windows[j].Index })
		v.Sessions = append(v.Sessions, ss)
	}
	sort.Slice(v.Sessions, func(i, j int) bool { return v.Sessions[i].Name < v.Sessions[j].Name })
	if e = v.Validate(); e != nil {
		return v, e
	}
	for _, q := range []struct {
		args []string
		raw  string
	}{{[]string{"list-sessions", "--json"}, rawS}, {[]string{"list-windows", "-a", "--json"}, rawW}, {[]string{"list-panes", "-a", "--json"}, rawP}} {
		again, e := r.call(q.args...)
		if e != nil {
			return v, e
		}
		if again != q.raw && !(q.args[0] == "list-windows" && sameWindowTopology(q.raw, again)) {
			return v, errors.New("rmux changed during capture, keeping last good snapshot")
		}
	}
	after, e := processQuery()
	if e != nil {
		return v, e
	}
	for id, pid := range roots {
		actual, e := r.call("display-message", "-p", "-t", id, "#{pane_pid}")
		if e != nil || strings.TrimSpace(actual) != strconv.Itoa(pid) || !sameRelevantProcesses(pid, processes, after) || ResolveJcode(r.Home, pid, after) != mappings[id] {
			return v, errors.New("pane/Jcode process identity changed during capture, keeping last good snapshot")
		}
	}
	return v, nil
}
func sameRelevantProcesses(root int, a, b map[int]Process) bool {
	relevant := func(m map[int]Process) map[int]Process {
		out := map[int]Process{}
		if p, ok := m[root]; ok {
			out[root] = p
		}
		for pid, p := range m {
			if filepath.Base(p.Executable) != "jcode" || !descendant(root, pid, m) {
				continue
			}
			seen := map[int]bool{}
			for pid > 0 && !seen[pid] {
				seen[pid] = true
				out[pid] = m[pid]
				if pid == root {
					break
				}
				pid = m[pid].Parent
			}
		}
		return out
	}
	x, y := relevant(a), relevant(b)
	if len(x) == 0 || len(x) != len(y) {
		return false
	}
	for pid, p := range x {
		q, ok := y[pid]
		if !ok || p.Parent != q.Parent || p.Executable != q.Executable || !p.Started.Equal(q.Started) {
			return false
		}
	}
	return true
}
func processTable() (map[int]Process, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,lstart=,comm=").Output()
	if e != nil {
		return nil, fmt.Errorf("process collection failed: %w", e)
	}
	return parseProcesses(string(out))
}
func parseProcesses(out string) (map[int]Process, error) {
	m := map[int]Process{}
	re := regexp.MustCompile(`^\s*([0-9]+)\s+([0-9]+)\s+(.{24})\s+(.+)$`)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := re.FindStringSubmatch(line)
		if f == nil {
			return nil, errors.New("invalid process table")
		}
		pid, e1 := strconv.Atoi(f[1])
		parent, e2 := strconv.Atoi(f[2])
		started, e3 := time.ParseInLocation("Mon Jan _2 15:04:05 2006", f[3], time.Local)
		if e1 != nil || e2 != nil || e3 != nil {
			return nil, errors.New("invalid process identity")
		}
		m[pid] = Process{Parent: parent, Executable: f[4], Started: started}
	}
	if len(m) == 0 {
		return nil, errors.New("empty process table")
	}
	return m, nil
}
func (r *Runtime) Save() (bool, error) {
	_, e := r.Store.Load()
	if e == nil && !r.ready() {
		return false, errors.New("saved state exists but server is not persistence-ready, restore first")
	}
	if e != nil && !errors.Is(e, unix.ENOENT) {
		return false, e
	}
	v, e := r.Capture()
	if e != nil {
		return false, e
	}
	changed, e := r.Store.Save(v)
	if e != nil {
		return false, e
	}
	if e = r.markReady(); e != nil {
		return changed, e
	}
	return changed, nil
}
func (r *Runtime) warn(format string, args ...any) {
	if r.Warnings != nil {
		fmt.Fprintf(r.Warnings, "rmux-state: "+format+"\n", args...)
	}
}
func (r *Runtime) cwd(path string) string {
	if i, e := os.Stat(path); e == nil && i.IsDir() {
		return path
	}
	fallback := r.Home
	if i, e := os.Stat(fallback); e != nil || !i.IsDir() {
		fallback = "/"
	}
	r.warn("missing cwd %q, using %q", path, fallback)
	return fallback
}
func trustedExecutable(path string) bool {
	real, e := filepath.EvalSymlinks(path)
	if e != nil || !filepath.IsAbs(real) {
		return false
	}
	parent, e := executableDir(filepath.Dir(real))
	if e != nil {
		return false
	}
	parent.Close()
	i, e := os.Stat(real)
	if e != nil || !i.Mode().IsRegular() || i.Mode().Perm()&0022 != 0 || i.Mode().Perm()&0111 == 0 {
		return false
	}
	var st unix.Stat_t
	if unix.Stat(real, &st) != nil {
		return false
	}
	return st.Uid == uint32(os.Getuid()) || st.Uid == 0
}
func (r *Runtime) jcodeBin() string {
	path := filepath.Join(r.Home, ".local", "bin", "jcode")
	if trustedExecutable(path) {
		return path
	}
	return ""
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func (r *Runtime) command(p Pane) string {
	if p.Jcode != "" {
		bin := r.jcodeBin()
		if bin != "" && validHistory(r.Home, p.Jcode) {
			return shellQuote(bin) + " --no-update --resume " + p.Jcode
		}
		r.warn("Jcode history or trusted executable unavailable for %s, starting shell", p.Jcode)
	}
	if trustedExecutable("/bin/zsh") {
		return "/bin/zsh -l"
	}
	return "/bin/sh -l"
}
func (r *Runtime) Restore() error {
	v, e := r.Store.Load()
	if errors.Is(e, unix.ENOENT) {
		if _, pe := r.Store.read("latest"); pe == nil {
			return e
		}
		return nil
	}
	if e != nil {
		return e
	}
	if r.ready() {
		return nil
	}
	sessions, _, e := readRows[sessionRow](r, "list-sessions", "--json")
	if e != nil && !errors.Is(e, ErrNoServer) {
		return e
	}
	if len(sessions) > 0 {
		return errors.New("refusing restore into running unmarked server, saved state retained")
	}
	for _, se := range v.Sessions {
		for _, w := range se.Windows {
			ids, _ := layoutIDs(w.Layout)
			for n, p := range w.Panes {
				if p.Jcode != "" && (r.jcodeBin() == "" || !validHistory(r.Home, p.Jcode)) {
					return errors.New("Jcode executable/history unavailable, saved state retained and autosave blocked")
				}
				if p.ID != ids[n] || p.Index != n+w.Panes[0].Index {
					return errors.New("noncanonical pane order cannot be restored safely")
				}
			}
		}
	}
	for _, se := range v.Sessions {
		for n, w := range se.Windows {
			target := fmt.Sprintf("%s:%d", se.Name, w.Index)
			first := w.Panes[0]
			var args []string
			if n == 0 {
				if r.Config != "" {
					args = append(args, "-f", r.Config)
				}
				args = append(args, "new-session", "-d", "-P", "-F", "#{pane_id}", "-s", se.Name, "-n", w.Name, "-x", strconv.Itoa(w.Width), "-y", strconv.Itoa(w.Height), "-c", r.cwd(first.CWD), r.command(first))
			} else {
				args = []string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-t", target, "-n", w.Name, "-c", r.cwd(first.CWD), r.command(first)}
			}
			output, e := r.call(args...)
			if e != nil {
				return fmt.Errorf("partial restore, autosave remains blocked: %w", e)
			}
			newID := strings.TrimSpace(output)
			if !paneID.MatchString(newID) {
				return errors.New("partial restore: invalid created pane ID")
			}
			activeID := ""
			if first.Active {
				activeID = newID
			}
			if n == 0 {
				rows, _, e := readRows[windowRow](r, "list-windows", "-t", se.Name, "--json")
				if e != nil || len(rows) != 1 {
					return errors.New("partial restore: cannot resolve initial window")
				}
				if rows[0].Index != w.Index {
					if _, e = r.call("move-window", "-s", fmt.Sprintf("%s:%d", se.Name, rows[0].Index), "-t", target); e != nil {
						return e
					}
				}
			}
			if _, e = r.call("set-window-option", "-t", target, "pane-base-index", strconv.Itoa(w.Panes[0].Index)); e != nil {
				return e
			}
			for _, p := range w.Panes[1:] {
				output, e = r.call("split-window", "-d", "-P", "-F", "#{pane_id}", "-t", newID, "-c", r.cwd(p.CWD), r.command(p))
				if e != nil {
					return e
				}
				newID = strings.TrimSpace(output)
				if !paneID.MatchString(newID) {
					return errors.New("partial restore: invalid split pane ID")
				}
				if p.Active {
					activeID = newID
				}
			}
			// Native custom layouts ignore serialized leaf IDs and apply leaf geometry in pane-index order.
			if _, e = r.call("select-layout", "-t", target, w.Layout); e != nil {
				return e
			}
			if _, e = r.call("select-pane", "-t", activeID); e != nil {
				return e
			}
		}
		for _, w := range se.Windows {
			if w.Active {
				if _, e = r.call("select-window", "-t", fmt.Sprintf("%s:%d", se.Name, w.Index)); e != nil {
					return e
				}
			}
		}
	}
	return r.markReady()
}
