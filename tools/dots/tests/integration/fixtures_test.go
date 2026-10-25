package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"dots/native/internal/desktop"
	"dots/native/internal/proxy"
	"dots/native/internal/uicli"
	"dots/native/tests/testutil/fakecmd"
)

type fakeRule = fakecmd.Rule
type fakeCall = fakecmd.Call

type fixture struct {
	t                     *testing.T
	base, root, home, bin string
	env                   []string
}

func TestMain(m *testing.M) {
	if code, handled := fakecmd.Run(os.Getenv("DESKTOP_FAKE_DIR")); handled {
		os.Exit(code)
	}
	if os.Getenv("DESKTOP_CLI") == "1" {
		os.Exit(desktop.Run(os.Getenv("DESKTOP_ROOT"), os.Args[1:]))
	}
	if os.Getenv("DOTS_UI_CLI") == "1" {
		args := os.Args[1:]
		if len(args) > 0 && args[0] == "ui" {
			args = args[1:]
		}
		os.Exit(uicli.Run(args))
	}
	if os.Getenv("DOTS_PROXY_CLI") == "1" {
		os.Exit(proxy.Run("", os.Args[1:]))
	}
	os.Exit(m.Run())
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, base: base, root: filepath.Join(base, "repo"), home: filepath.Join(base, "home"), bin: filepath.Join(base, "bin")}
	coverage := filepath.Join(base, "coverage")
	for _, path := range []string{f.root, f.home, f.bin, coverage} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.env = []string{"HOME=" + f.home, "PATH=" + f.bin, "XDG_CONFIG_HOME=" + filepath.Join(f.home, ".config"), "XDG_CACHE_HOME=" + filepath.Join(f.home, ".cache"), "XDG_STATE_HOME=" + filepath.Join(f.home, ".local/state"), "DESKTOP_CLI=1", "DESKTOP_ROOT=" + f.root, "DESKTOP_FAKE_DIR=" + f.bin, "GOCOVERDIR=" + coverage, "GORACE=atexit_sleep_ms=0"}
	return f
}

func (f *fixture) fake(name string, rules ...fakeRule) {
	f.t.Helper()
	fakecmd.Install(f.t, f.bin, name, rules...)
}

func (f *fixture) calls() []fakecmd.Call {
	f.t.Helper()
	return fakecmd.Calls(f.t, f.bin)
}

func (f *fixture) write(path, content string, mode os.FileMode) {
	f.t.Helper()
	put(f.t, path, content)
	if err := os.Chmod(path, mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) repo(name string) string {
	f.t.Helper()
	path := filepath.Join(f.base, name)
	f.write(filepath.Join(path, ".git/HEAD"), "ref: refs/heads/main\n", 0600)
	if err := os.MkdirAll(filepath.Join(path, ".git/objects"), 0700); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func (f *fixture) registry(paths ...string) {
	f.t.Helper()
	rows := []map[string]string{}
	for index, path := range paths {
		rows = append(rows, map[string]string{"id": "fixture-" + strconv.Itoa(index), "path": path})
	}
	data, err := json.Marshal(map[string]any{"version": 1, "projects": rows})
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(filepath.Join(f.home, ".config/dots/projects.json"), string(data), 0600)
}

func (f *fixture) run(input string, args ...string) (string, string, int) {
	f.t.Helper()
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = f.env
	cmd.Dir = f.root
	cmd.Stdin = strings.NewReader(input)
	var out, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	code := 0
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			f.t.Fatal(err)
		}
	}
	return out.String(), diagnostic.String(), code
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func allRule(output string, code int) fakeRule {
	return fakeRule{Args: []string{}, Prefix: true, Output: output, Code: code}
}

func envSet(env []string, values ...string) []string {
	out := append([]string{}, env...)
	for _, value := range values {
		key, _, _ := strings.Cut(value, "=")
		filtered := out[:0]
		for _, old := range out {
			if !strings.HasPrefix(old, key+"=") {
				filtered = append(filtered, old)
			}
		}
		out = append(filtered, value)
	}
	return out
}
