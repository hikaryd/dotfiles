package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"dots/native/internal/testpty"
)

func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "kubectl" && os.Getenv("METRICS_FAKE_DIR") != "" {
		fakeForward()
		os.Exit(0)
	}
	if os.Getenv("DOTS_METRICS_CLI") == "1" {
		name := filepath.Base(os.Args[0])
		if name == "km" || name == "kma" {
			os.Exit(RunAlias(name, os.Args[1:]))
		}
		os.Exit(Run(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeForward() {
	dir := os.Getenv("METRICS_FAKE_DIR")
	data, _ := json.Marshal(os.Args[1:])
	os.WriteFile(filepath.Join(dir, "args.json"), data, 0600)
	os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	fmt.Fprintln(os.Stderr, "PRIVATE_FORWARD_DIAGNOSTIC")
	if os.Getenv("METRICS_NO_READY") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(98)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	remote := strings.TrimPrefix(os.Args[len(os.Args)-1], ":")
	if target := os.Getenv("METRICS_TARGET_PORT"); target != "" {
		remote = target
	}
	fmt.Printf("Forwarding from 127.0.0.1:%d -> %s\n", port, remote)
	http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(responseFor(r, "42")) }))
}

func cli(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "DOTS_METRICS_CLI=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0")
	return cmd
}

func fakeKubectl(t *testing.T) (profile, string) {
	t.Helper()
	dir := t.TempDir()
	exe, _ := os.Executable()
	if err := os.Symlink(exe, filepath.Join(dir, "kubectl")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("METRICS_FAKE_DIR", dir)
	p := fixtureProfile()
	p.URL = ""
	p.Forward = &forward{Kubeconfig: filepath.Join(dir, "fixture.yaml"), Context: "example-context", Namespace: "observability", Resource: "svc/metrics", RemotePort: 19090}
	os.WriteFile(p.Forward.Kubeconfig, []byte("fixture"), 0600)
	return p, dir
}

func readPID(t *testing.T, dir string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(dir, "pid"))
		if err == nil {
			pid, _ := strconv.Atoi(string(data))
			if pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fake port-forward did not start")
	return 0
}

func TestOwnedForwardUsesPinnedArgvLoopbackAndReapsChild(t *testing.T) {
	t.Setenv("METRICS_TARGET_PORT", "19091") // service port != pod targetPort
	p, dir := fakeKubectl(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, close, err := connect(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.fetch(ctx, metric{ID: "test", Query: "up"}, time.Hour, time.Minute, time.Now())
	if err != nil || *current(data.Series[0]) != 42 {
		close()
		t.Fatalf("forward fetch: %v", err)
	}
	pid := readPID(t, dir)
	close()
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("owned child remains: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "args.json"))
	var args []string
	json.Unmarshal(raw, &args)
	want := []string{"--kubeconfig=" + p.Forward.Kubeconfig, "--context=example-context", "--namespace=observability", "port-forward", "--address=127.0.0.1", "svc/metrics", ":19090"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("argv: %v", args)
	}
}

func TestCLIPipeJSONAndPartialFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(responseFor(r, "12.5")) }))
	defer server.Close()
	p := fixtureProfile()
	p.URL = server.URL
	path := saveConfig(t, p)
	out, err := cli(t, "--config", path, "--metric", "rps", "--json").CombinedOutput()
	if err != nil || strings.Contains(string(out), "\x1b") {
		t.Fatalf("JSON CLI: %s %v", out, err)
	}
	var data snapshot
	if json.Unmarshal(out, &data) != nil || data.Metric != "rps" || *current(data.Series[0]) != 12.5 {
		t.Fatalf("JSON: %s", out)
	}
	out, err = cli(t, "--config", path).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Получены данные: 8 из 8") || strings.Contains(string(out), "\x1b") {
		t.Fatalf("pipe snapshot: %s %v", out, err)
	}
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); fmt.Fprint(w, "PRIVATE_TOKEN") }))
	defer failed.Close()
	p.URL = failed.URL
	path = saveConfig(t, p)
	out, err = cli(t, "--config", path, "--metric", "rps", "--json").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "HTTP 503") || strings.Contains(string(out), "PRIVATE") || strings.Contains(string(out), failed.URL) {
		t.Fatalf("failure: %s %v", out, err)
	}
}

func TestLiveOverviewDetailBreakdownPauseResizeAndRestore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(responseFor(r, "12.5")) }))
	defer server.Close()
	p := fixtureProfile()
	p.URL = server.URL
	path := saveConfig(t, p)
	terminal := testpty.Start(t, cli(t, "--config", path))
	terminal.Screen("Получены данные: 8 из 8")
	terminal.Send("\x1b[200~q123b \x1b[201~") // paste is not a set of hotkeys
	// Moving the selection does not start a request; Enter is the explicit drill-down.
	terminal.Send("\x1b[B\r")
	terminal.Screen("Ошибки сервера · 5xx")
	terminal.Screen("В абсолютных числах: 12,5 запр./с")
	terminal.Send("b")
	terminal.Screen("Ошибки 5xx по сервисам")
	terminal.Send("?")
	terminal.Screen("УПРАВЛЕНИЕ")
	terminal.Send("\x1b[6~\x1b[6~\x1b[6~")
	terminal.Screen("PromQL:")
	terminal.Send("?")
	terminal.Send(" ")
	terminal.Screen("ПАУЗА")
	terminal.Resize(80, 24)
	terminal.Send("\x1b")
	terminal.Screen("Ошибки сервера · 5xx")
	terminal.Send("\x1b")
	terminal.Send("4")
	terminal.Screen("[4 Состояние]")
	terminal.Send("q")
	terminal.Wait(0)
	if strings.Contains(terminal.Text(), server.URL) {
		t.Fatal("connection coordinate printed")
	}
}

func TestCancelDuringFirstHTTPFetchAndPortForwardReadiness(t *testing.T) {
	t.Run("http", func(t *testing.T) {
		started, canceled := make(chan struct{}, 4), make(chan struct{}, 4)
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.ParseForm() // Consume POST body so net/http can observe client disconnect.
			started <- struct{}{}
			select {
			case <-r.Context().Done():
				canceled <- struct{}{}
			case <-release:
			}
		}))
		defer server.Close()
		defer close(release)
		p := fixtureProfile()
		p.URL = server.URL
		path := saveConfig(t, p)
		terminal := testpty.Start(t, cli(t, "--config", path))
		terminal.Screen("Обновление…")
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("no request")
		}
		terminal.Send("\x03")
		terminal.Wait(0)
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("HTTP request not canceled")
		}
	})
	t.Run("forward", func(t *testing.T) {
		p, dir := fakeKubectl(t)
		t.Setenv("METRICS_NO_READY", "1")
		terminal := testpty.Start(t, cli(t, "--config", saveConfig(t, p)))
		terminal.Screen("Обновление…")
		pid := readPID(t, dir)
		terminal.Send("q")
		terminal.Wait(0)
		if syscall.Kill(pid, 0) != syscall.ESRCH {
			t.Fatal("forward left running")
		}
		if strings.Contains(terminal.Text(), "PRIVATE_FORWARD") {
			t.Fatal("kubectl stderr leaked")
		}
	})
}

func TestTerminalHangupReapsOwnedForward(t *testing.T) {
	p, dir := fakeKubectl(t)
	t.Setenv("METRICS_NO_READY", "1")
	cmd := cli(t, "--config", saveConfig(t, p))
	terminal := testpty.Start(t, cmd)
	pid := readPID(t, dir)
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	terminal.Wait(0)
	if syscall.Kill(pid, 0) != syscall.ESRCH {
		t.Fatal("terminal hangup left the owned forward running")
	}
}

func TestPreviewAndCheckDoNotContactCluster(t *testing.T) {
	p, dir := fakeKubectl(t)
	path := saveConfig(t, p)
	for _, args := range [][]string{{"--preview", "--config", "/missing/private/file", "--scenario", "degraded"}, {"check", "--config", path}, {"--config", path, "--list"}} {
		out, err := cli(t, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("local command: %s %v", out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "pid")); !os.IsNotExist(err) {
		t.Fatal("offline command contacted kubectl")
	}
}
