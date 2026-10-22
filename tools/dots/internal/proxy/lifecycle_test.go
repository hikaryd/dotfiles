package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("DOTS_PROXY_CLI") == "1" {
		os.Exit(Run("", os.Args[1:]))
	}
	if fixture := os.Getenv("DOTS_PROXY_TEST_FIXTURE"); fixture != "" {
		switch filepath.Base(os.Args[0]) {
		case "launchctl":
			fakeLaunchctl(fixture)
			os.Exit(0)
		case "xray":
			if os.Getenv("DOTS_PROXY_TEST_XRAY_FAIL") == "1" {
				fmt.Fprintln(os.Stderr, "fixture-password-do-not-log")
				os.Exit(23)
			}
			if os.Getenv("DOTS_PROXY_TEST_ROUTES") == "1" {
				serveFixtureRoute(fixture)
				os.Exit(0)
			}
			if len(os.Args) > 3 && strings.Contains(os.Args[3], ".probe-") {
				data, _ := os.ReadFile(os.Args[3])
				var cfg struct{ Inbounds []struct{ Port int } }
				if json.Unmarshal(data, &cfg) != nil || len(cfg.Inbounds) == 0 {
					os.Exit(2)
				}
				listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(cfg.Inbounds[0].Port))
				if err != nil {
					os.Exit(3)
				}
				_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, "isolated probe")
				}))
				os.Exit(0)
			}
			b, _ := json.Marshal(os.Args)
			if os.WriteFile(filepath.Join(fixture, "xray-argv"), b, 0600) != nil {
				os.Exit(2)
			}
			if os.WriteFile(filepath.Join(fixture, "child-pid"), []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
				os.Exit(2)
			}
			for {
				time.Sleep(time.Hour)
			}
		case "ps":
			b, err := os.ReadFile(filepath.Join(fixture, "xray-argv"))
			if err != nil {
				os.Exit(1)
			}
			pid, err := os.ReadFile(filepath.Join(fixture, "child-pid"))
			if err != nil || len(os.Args) < 3 || os.Args[2] != string(pid) {
				os.Exit(1)
			}
			n, _ := strconv.Atoi(string(pid))
			if syscall.Kill(n, 0) != nil {
				os.Exit(1)
			}
			var args []string
			if json.Unmarshal(b, &args) != nil {
				os.Exit(1)
			}
			for i, a := range args {
				args[i] = strconv.Quote(a)
			}
			fmt.Println(strings.Join(args, " "))
			os.Exit(0)
		case "pi":
			if os.Getenv("DOTS_PROXY_PI_BLOCK") == "1" {
				os.WriteFile(filepath.Join(fixture, "pi-pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
				for {
					time.Sleep(time.Hour)
				}
			}
			env := map[string]string{}
			for _, k := range proxyKeys {
				env[k] = os.Getenv(k)
			}
			in, _ := io.ReadAll(os.Stdin)
			_ = json.NewEncoder(os.Stdout).Encode(object{"args": os.Args[1:], "env": env, "stdin": string(in)})
			os.Exit(7)
		}
	}
	os.Exit(m.Run())
}
func fakeCommands(t *testing.T) string {
	t.Helper()
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		t.Skip("socket-ownership fixtures require lsof")
	}
	fixture := t.TempDir()
	bin := filepath.Join(fixture, "fake commands")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xray", "ps", "pi", "launchctl"} {
		if err := os.Symlink(exe, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	// Socket ownership is checked against real fixture sockets, not mocked PIDs.
	if err := os.Symlink(lsof, filepath.Join(bin, "lsof")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("DOTS_PROXY_TEST_FIXTURE", fixture)
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	return fixture
}
func fixtureVLESS(t *testing.T) object {
	t.Helper()
	s, err := parseVLESS("vless://fixture-id@example.test:443?type=xhttp&security=reality&pbk=fixture-key#Fixture")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func cleanupChild(t *testing.T, c *controller, fixture string) {
	t.Helper()
	t.Cleanup(func() {
		if b, err := os.ReadFile(filepath.Join(fixture, "child-pid")); err == nil {
			pid, _ := strconv.Atoi(string(b))
			if pid > 1 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		_ = c.removePID()
	})
}
func TestOwnedCommand(t *testing.T) {
	cfg := "/fixture home/.config/xray-pi/config.json"
	for _, command := range []string{`/bin/xray run -c "/fixture home/.config/xray-pi/config.json"`, `/bin/xray run -c /fixture\ home/.config/xray-pi/config.json`, `xray run -c /fixture home/.config/xray-pi/config.json`} {
		if !ownsCommand(command, cfg) {
			t.Fatalf("rejected %s", command)
		}
	}
	for _, command := range []string{"", `other run -c "` + cfg + `"`, `xray run -c "` + cfg + `.backup"`, `xray other "` + cfg + `"`, `xray run --label "` + cfg + `"`, `xray run -c /different/config.json`} {
		if ownsCommand(command, cfg) {
			t.Fatalf("accepted %s", command)
		}
	}
}
func TestLifecycleAndRunCLI(t *testing.T) {
	fixture := fakeCommands(t)
	c := testController(t)
	cleanupChild(t, c, fixture)
	if err := c.start(fixtureVLESS(t)); err != nil {
		t.Fatal(err)
	}
	pid, err := c.readPID()
	if err != nil || pid < 2 {
		t.Fatalf("pid %d %v", pid, err)
	}
	if !c.owned(pid) {
		t.Fatal("fake xray not recognized")
	}
	before, _ := os.ReadFile(c.path("config.json"))
	if err := c.start(object{"name": "do not overwrite"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(c.path("config.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("already-running config overwritten")
	}
	for _, name := range []string{"config.json", "xray.pid", "controller.lock"} {
		st, err := os.Stat(c.path(name))
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatalf("%s %v %v", name, st, err)
		}
	}
	c.out.(*bytes.Buffer).Reset()
	c.rawIn = strings.NewReader("fixture stdin\n")
	code, err := c.runCLI([]string{"exec", "argument with spaces", "$(not-a-command)"})
	if err != nil || code != 7 {
		t.Fatalf("run %d %v", code, err)
	}
	output := c.out.(*bytes.Buffer).String()
	_, body, _ := strings.Cut(output, "\n")
	var result struct {
		Args  []string
		Env   map[string]string
		Stdin string
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("child output %q %v", body, err)
	}
	if strings.Join(result.Args, "|") != "exec|argument with spaces|$(not-a-command)" || result.Stdin != "fixture stdin\n" {
		t.Fatalf("child argv/stdin %#v", result)
	}
	for _, k := range proxyKeys {
		if result.Env[k] != c.envValue(k) {
			t.Fatalf("env %s=%s", k, result.Env[k])
		}
	}
	if err := c.stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.path("xray.pid")); !os.IsNotExist(err) {
		t.Fatal("PID retained after stop")
	}
	if c.requireRunning() == nil {
		t.Fatal("allowed run after stop")
	}
}
func TestStartFailureWithholdsSecrets(t *testing.T) {
	fakeCommands(t)
	t.Setenv("DOTS_PROXY_TEST_XRAY_FAIL", "1")
	c := testController(t)
	err := c.start(fixtureVLESS(t))
	if err == nil {
		t.Fatal("success on child failure")
	}
	text := err.Error() + c.out.(*bytes.Buffer).String() + c.errOut.(*bytes.Buffer).String()
	if strings.Contains(text, "fixture-password-do-not-log") || strings.Contains(text, "fixture-id") {
		t.Fatal("secret leaked")
	}
	if _, err := os.Stat(c.path("xray.pid")); !os.IsNotExist(err) {
		t.Fatal("PID created for failed child")
	}
}
func TestPIDWriteFailureCleansUp(t *testing.T) {
	fixture := fakeCommands(t)
	c := testController(t)
	cleanupChild(t, c, fixture)
	// A child creates the obstructing directory after the initial PID read.
	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(fixture, "child-pid")); err == nil {
				_ = os.Mkdir(c.path("xray.pid"), 0700)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	err := c.start(fixtureVLESS(t))
	<-done
	if err == nil || !strings.Contains(err.Error(), "Could not save xray PID") {
		t.Fatalf("error %v", err)
	}
	b, readErr := os.ReadFile(filepath.Join(fixture, "child-pid"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, _ := strconv.Atoi(string(b))
	if pidRunning(pid) {
		t.Fatal("orphaned fake xray after PID write failure")
	}
}
func TestStaleAndUnownedPIDsNeverSignaled(t *testing.T) {
	fakeCommands(t)
	c := testController(t)
	for _, content := range []string{"bad\n", "0\n", "-1\n", "1\n", strconv.Itoa(os.Getpid()) + "\n"} {
		if err := atomicWrite(c.path("xray.pid"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := c.stop(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(c.path("xray.pid")); !os.IsNotExist(err) {
			t.Fatal("stale PID not removed")
		}
	}
	if !strings.Contains(c.errOut.(*bytes.Buffer).String(), "Refusing to stop") {
		t.Fatal("missing ownership warning")
	}
}
func TestMissingXray(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := testController(t)
	if err := c.start(fixtureVLESS(t)); err == nil || !strings.Contains(err.Error(), "xray not found") {
		t.Fatalf("%v", err)
	}
}
func TestExportedRunInFixtureProcess(t *testing.T) {
	// Validate the public entry point with real process stdout/stderr and HOME.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestPublicRunChild$")
	cmd.Env = append(os.Environ(), "DOTS_PROXY_PUBLIC_CHILD=1", "HOME="+t.TempDir(), "PI_PROXY_PORT=10810")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v %s", err, output)
	}
	if !bytes.Contains(output, []byte("export WS_PROXY=\"http://127.0.0.1:10810\"")) {
		t.Fatalf("%s", output)
	}
}
func TestPublicRunChild(t *testing.T) {
	if os.Getenv("DOTS_PROXY_PUBLIC_CHILD") != "1" {
		return
	}
	os.Exit(Run("", []string{"pi", "--print-env"}))
}
func TestControllerLockBoundedAndReleased(t *testing.T) {
	c := testController(t)
	unlock, err := c.lock()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = c.lock()
	elapsed := time.Since(start)
	unlock()
	if err == nil || !strings.Contains(err.Error(), "Timed out") || elapsed > 7*time.Second {
		t.Fatalf("unbounded lock: %v %v", elapsed, err)
	}
	unlock, err = c.lock()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
