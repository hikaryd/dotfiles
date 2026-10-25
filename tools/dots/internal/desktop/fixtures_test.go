package desktop

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"dots/native/tests/testutil/fakecmd"
)

type fakeRule = fakecmd.Rule
type fakeCall = fakecmd.Call

func TestMain(m *testing.M) {
	if code, handled := fakecmd.Run(os.Getenv("DESKTOP_FAKE_DIR")); handled {
		os.Exit(code)
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
	fakecmd.Install(f.t, f.bin, name, rules...)
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
	return fakecmd.Calls(f.t, f.bin)
}
func (f *fixture) localEnv() {
	f.t.Helper()
	f.t.Setenv("DESKTOP_FAKE_DIR", f.bin)
	for _, key := range []string{"HERDR_ENV", "HERDR_SOCKET_PATH", "DOTS_ROOT", "DOTS_PRIVATE_ZSH", "TMUX", "RMUX", "RMUX_PANE", "DOTS_SOURCE_PANE", "DOTS_MUX_CLIENT"} {
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
