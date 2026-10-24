package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"dots/native/internal/tui"
)

type fakeRule struct {
	Args    []string
	Prefix  bool
	Output  string
	Outputs []string
	Stderr  string
	Code    int
	Block   bool
}
type fakeCall struct {
	Name  string
	Args  []string
	Input string
	Env   map[string]string
}

func TestMain(m *testing.M) {
	if dir := os.Getenv("DESKTOP_FAKE_DIR"); dir != "" {
		name := filepath.Base(os.Args[0])
		if b, e := os.ReadFile(filepath.Join(dir, name+".json")); e == nil {
			var rules []fakeRule
			if json.Unmarshal(b, &rules) != nil {
				os.Exit(98)
			}
			var input []byte
			if !tui.IsTerminal(os.Stdin) {
				input, _ = io.ReadAll(io.LimitReader(os.Stdin, maxBytes+1))
			}
			call := fakeCall{Name: name, Args: os.Args[1:], Input: string(input), Env: map[string]string{"DOTS_ROOT": os.Getenv("DOTS_ROOT"), "DOTS_ZSH_BIN": os.Getenv("DOTS_ZSH_BIN"), "PATH": os.Getenv("PATH"), "GIT_OPTIONAL_LOCKS": os.Getenv("GIT_OPTIONAL_LOCKS"), "FZF_DEFAULT_OPTS": os.Getenv("FZF_DEFAULT_OPTS"), "FZF_DEFAULT_OPTS_FILE": os.Getenv("FZF_DEFAULT_OPTS_FILE")}}
			f, e := os.OpenFile(filepath.Join(dir, "calls"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if e != nil {
				os.Exit(98)
			}
			json.NewEncoder(f).Encode(call)
			f.Close()
			for i, r := range rules {
				match := reflect.DeepEqual(r.Args, os.Args[1:])
				if r.Prefix && len(os.Args)-1 >= len(r.Args) {
					match = reflect.DeepEqual(r.Args, os.Args[1:1+len(r.Args)])
				}
				if match {
					out := r.Output
					if len(r.Outputs) > 0 {
						counter := filepath.Join(dir, fmt.Sprintf("%s.%d.counter", name, i))
						b, _ := os.ReadFile(counter)
						n, _ := strconv.Atoi(string(b))
						out = r.Outputs[min(n, len(r.Outputs)-1)]
						os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0600)
					}
					if r.Block {
						os.WriteFile(filepath.Join(dir, "blocked-"+name), []byte("ready"), 0600)
						time.Sleep(time.Minute)
					}
					fmt.Fprint(os.Stdout, out)
					fmt.Fprint(os.Stderr, r.Stderr)
					os.Exit(r.Code)
				}
			}
			fmt.Fprintln(os.Stderr, "unexpected fixture command")
			os.Exit(97)
		}
	}
	if os.Getenv("DESKTOP_CLI") == "1" {
		os.Exit(Run(os.Getenv("DESKTOP_ROOT"), os.Args[1:]))
	}
	os.Exit(m.Run())
}

type fixture struct {
	t                     *testing.T
	base, root, home, bin string
	env                   []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	base := resolve(t.TempDir())
	f := &fixture{t: t, base: base, root: filepath.Join(base, "repo"), home: filepath.Join(base, "home"), bin: filepath.Join(base, "bin")}
	coverage := filepath.Join(base, "coverage")
	for _, p := range []string{f.root, f.home, f.bin, coverage} {
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	f.env = []string{"HOME=" + f.home, "PATH=" + f.bin, "XDG_CONFIG_HOME=" + filepath.Join(f.home, ".config"), "XDG_CACHE_HOME=" + filepath.Join(f.home, ".cache"), "XDG_STATE_HOME=" + filepath.Join(f.home, ".local/state"), "DESKTOP_CLI=1", "DESKTOP_ROOT=" + f.root, "DESKTOP_FAKE_DIR=" + f.bin, "GOCOVERDIR=" + coverage, "GORACE=atexit_sleep_ms=0"}
	return f
}
func (f *fixture) fake(name string, rules ...fakeRule) {
	f.t.Helper()
	exe, e := os.Executable()
	if e != nil {
		f.t.Fatal(e)
	}
	p := filepath.Join(f.bin, name)
	if !exists(p) {
		if e = os.Symlink(exe, p); e != nil {
			f.t.Fatal(e)
		}
	}
	b, _ := json.Marshal(rules)
	f.write(p+".json", string(b), 0600)
}
func (f *fixture) write(p, s string, mode os.FileMode) {
	f.t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		f.t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), mode); e != nil {
		f.t.Fatal(e)
	}
}
func (f *fixture) run(input string, args ...string) (string, string, int) {
	f.t.Helper()
	exe, e := os.Executable()
	if e != nil {
		f.t.Fatal(e)
	}
	cmd := exec.Command(exe, args...)
	// Fixtures must never acquire the developer's controlling terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = f.env
	cmd.Dir = f.root
	cmd.Stdin = strings.NewReader(input)
	var out, err bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &err
	e = cmd.Run()
	code := 0
	if e != nil {
		if ee, ok := e.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			f.t.Fatal(e)
		}
	}
	return out.String(), err.String(), code
}
func (f *fixture) calls() []fakeCall {
	f.t.Helper()
	b, _ := os.ReadFile(filepath.Join(f.bin, "calls"))
	calls := []fakeCall{}
	d := json.NewDecoder(bytes.NewReader(b))
	for {
		var c fakeCall
		e := d.Decode(&c)
		if e == io.EOF {
			break
		}
		if e != nil {
			f.t.Fatal(e)
		}
		calls = append(calls, c)
	}
	return calls
}
func (f *fixture) localEnv() {
	f.t.Helper()
	f.t.Setenv("DESKTOP_FAKE_DIR", f.bin)
	for _, key := range []string{"HERDR_ENV", "HERDR_SOCKET_PATH", "DOTS_ROOT", "DOTS_PRIVATE_ZSH", "TMUX"} {
		f.t.Setenv(key, "")
	}
	for _, s := range f.env {
		k, v, _ := strings.Cut(s, "=")
		if !strings.HasPrefix(k, "DESKTOP_") {
			f.t.Setenv(k, v)
		}
	}
}
func (f *fixture) repo(name string) string {
	p := filepath.Join(f.base, name)
	f.write(filepath.Join(p, ".git/HEAD"), "ref: refs/heads/main\n", 0600)
	if e := os.MkdirAll(filepath.Join(p, ".git/objects"), 0700); e != nil {
		f.t.Fatal(e)
	}
	return p
}
func (f *fixture) registry(paths ...string) {
	rows := []map[string]string{}
	for _, p := range paths {
		rows = append(rows, map[string]string{"id": projectID(p), "path": p})
	}
	b, _ := json.Marshal(map[string]any{"version": 1, "projects": rows})
	f.write(filepath.Join(f.home, ".config/dots/projects.json"), string(b), 0600)
}
func allRule(out string, code int) fakeRule {
	return fakeRule{Args: []string{}, Prefix: true, Output: out, Code: code}
}
