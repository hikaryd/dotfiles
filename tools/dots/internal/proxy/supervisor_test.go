package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func startMonitored(t *testing.T) (*controller, string, int, []byte) {
	t.Helper()
	c, fixture, servers := startupFixture(t)
	if err := c.start(servers[0]); err != nil {
		t.Fatal(err)
	}
	pid, _ := c.readPID()
	cfg, _ := os.ReadFile(c.path("config.json"))
	return c, fixture, pid, cfg
}
func sameRoute(t *testing.T, c *controller, pid int, cfg []byte) {
	t.Helper()
	gotPID, _ := c.readPID()
	gotConfig, _ := os.ReadFile(c.path("config.json"))
	if gotPID != pid || !pidRunning(pid) || !bytes.Equal(gotConfig, cfg) {
		t.Fatal("live route was disrupted")
	}
}
func streamFixture(t *testing.T, c *controller) func(string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	reader := bufio.NewReader(conn)
	fmt.Fprint(conn, "GET /stream HTTP/1.1\r\nHost: fixture.test\r\n\r\n")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	echo := func(s string) {
		t.Helper()
		conn.SetDeadline(time.Now().Add(time.Second))
		fmt.Fprintln(conn, s)
		line, err := reader.ReadString('\n')
		if err != nil || line != s+"\n" {
			t.Fatalf("held-open connection lost: %q %v", line, err)
		}
	}
	echo("before")
	return echo
}
func setHealth(t *testing.T, fixture, health string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fixture, "health"), []byte(health), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestSupervisorAdoptsHealthyProcess(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	echo := streamFixture(t, c)
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	sameRoute(t, c, pid, cfg)
	echo("after adoption")
	if err := c.requireReady(); err != nil {
		t.Fatal(err)
	}
	requests, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if strings.Contains(string(requests), " true ") {
		t.Fatal("healthy adoption probed subscription candidates")
	}
}

func TestSupervisorChecksIdentityOncePerBoundary(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	sameRoute(t, c, pid, cfg)
	data, err := os.ReadFile(filepath.Join(fixture, "ps-calls"))
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(calls) != 2 {
		t.Fatalf("healthy maintenance launched %d ps checks, want one before and one after probing", len(calls))
	}
	requests, err := os.ReadFile(filepath.Join(fixture, "requests"))
	if err != nil || string(requests) != "fast.test false /country\nfast.test false /models\n" {
		t.Fatalf("maintenance repeated network checks or probed candidates: %q %v", requests, err)
	}
}

func TestSupervisorRejectsConfigChangeDuringProbe(t *testing.T) {
	c, fixture, _, cfg := startMonitored(t)
	var changed object
	if err := json.Unmarshal(cfg, &changed); err != nil {
		t.Fatal(err)
	}
	changed["log"] = object{"loglevel": "debug"}
	if err := writeJSON(filepath.Join(fixture, "rewrite-config"), changed); err != nil {
		t.Fatal(err)
	}
	if c.maintain(&monitorState{}) == nil {
		t.Fatal("configuration changed during probe but was certified ready")
	}
	if _, err := os.Stat(c.path("selection.json")); !os.IsNotExist(err) {
		t.Fatal("readiness published for a different route", err)
	}
}
func TestSupervisorAdoptsWithoutSelection(t *testing.T) {
	c, _, pid, cfg := startMonitored(t)
	if err := c.saveServers([]object{startupServer(t, "blocked.test")}); err != nil {
		t.Fatal(err)
	}
	os.Remove(c.path("selection.json"))
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	sameRoute(t, c, pid, cfg)
	if err := c.requireReady(); err != nil {
		t.Fatal("actual route absent from subscription not adopted", err)
	}
}
func TestSupervisorToleratesTransientFailures(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	state := &monitorState{}
	if err := c.maintain(state); err != nil {
		t.Fatal(err)
	}
	setHealth(t, fixture, "fail")
	for i := 1; i <= 2; i++ {
		if c.maintain(state) == nil || state.failures != i {
			t.Fatalf("failure %d not counted: %+v", i, state)
		}
		sameRoute(t, c, pid, cfg)
	}
	setHealth(t, fixture, "")
	if err := c.maintain(state); err != nil || state.failures != 0 {
		t.Fatal("success did not reset failures", err, state)
	}
	// The third failure permits first-working-route recovery, never an earlier one.
	if err := c.saveServers([]object{startupServer(t, "second.test")}); err != nil {
		t.Fatal(err)
	}
	setHealth(t, fixture, "fail")
	for i := 0; i < 2; i++ {
		_ = c.maintain(state)
		sameRoute(t, c, pid, cfg)
	}
	_ = c.maintain(state)
	next, _ := c.readPID()
	if next == pid {
		t.Fatal("third consecutive failure did not permit recovery")
	}
	if c.requireReady() == nil {
		t.Fatal("failed live replacement certified ready")
	}
}
func TestSupervisorRecoversDeadProcess(t *testing.T) {
	c, _, pid, _ := startMonitored(t)
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	deadline := time.Now().Add(time.Second)
	for pidRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	servers := []object{startupServer(t, "ru.test"), startupServer(t, "unknown.test"), startupServer(t, "blocked.test"), startupServer(t, "second.test")}
	if err := c.saveServers(servers); err != nil {
		t.Fatal(err)
	}
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	next, _ := c.readPID()
	if next == pid || !c.matchesRunning(servers[3]) || c.requireReady() != nil {
		t.Fatal("dead process not recovered to verified route")
	}
}
func TestSupervisorRejectsRUImmediately(t *testing.T) {
	c, fixture, pid, _ := startMonitored(t)
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	// Even a candidate that passes its isolated check must be stopped if the
	// replacement's real listener is also confirmed RU.
	if err := c.saveServers([]object{startupServer(t, "fast.test")}); err != nil {
		t.Fatal(err)
	}
	setHealth(t, fixture, "RU")
	_ = c.maintain(&monitorState{})
	next, _ := c.readPID()
	if pidRunning(pid) || pidRunning(next) || c.requireReady() == nil {
		t.Fatal("confirmed RU process kept alive or ready")
	}
}
func TestSupervisorPreservesLiveProcessWhenNoReplacementExists(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	state := &monitorState{}
	if err := c.maintain(state); err != nil {
		t.Fatal(err)
	}
	if err := c.saveServers([]object{startupServer(t, "blocked.test")}); err != nil {
		t.Fatal(err)
	}
	setHealth(t, fixture, "fail")
	for i := 0; i < 3; i++ {
		_ = c.maintain(state)
	}
	sameRoute(t, c, pid, cfg)
	if c.requireReady() == nil {
		t.Fatal("persistent failure republished readiness")
	}
	data, _ := os.ReadFile(c.path("selection.json"))
	var hint selection
	if json.Unmarshal(data, &hint) != nil || hint.Server == "" {
		t.Fatal("last server hint destroyed")
	}
	probes, _ := filepath.Glob(c.path(".probe-*"))
	if len(probes) != 0 {
		t.Fatal("leaked probes", probes)
	}
}
func TestSupervisorRefusesForeignListener(t *testing.T) {
	c, _, _ := startupFixture(t)
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := []byte("foreign-config-must-survive")
	os.WriteFile(c.path("config.json"), cfg, 0600)
	os.WriteFile(c.path("xray.pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	if c.maintain(&monitorState{}) == nil || c.requireReady() == nil {
		t.Fatal("foreign listener accepted")
	}
	after, _ := os.ReadFile(c.path("config.json"))
	if !bytes.Equal(after, cfg) || !pidRunning(os.Getpid()) {
		t.Fatal("foreign state altered")
	}
	pid, _ := c.readPID()
	if pid != os.Getpid() {
		t.Fatal("foreign PID file changed")
	}
}
func TestSupervisorCancellationAndDuplicate(t *testing.T) {
	c, _, pid, cfg := startMonitored(t)
	echo := streamFixture(t, c)
	unlock, err := c.lockFile("supervisor.lock", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.daemon() == nil {
		t.Fatal("duplicate supervisor accepted")
	}
	unlock()
	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	// Cancel a running monitor after its initial adoption, not only before startup.
	done := make(chan error, 1)
	go func() { done <- c.daemon() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(c.path("selection.json")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("daemon never adopted route")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("cancellation %v", err)
	}
	sameRoute(t, c, pid, cfg)
	echo("after cancellation")
}
func TestRequireReadyIsLocalAndFailClosed(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	if c.requireReady() == nil {
		t.Fatal("unchecked route accepted")
	}
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(c.path("selection.json"))
	var valid selection
	if json.Unmarshal(data, &valid) != nil {
		t.Fatal("invalid selection")
	}
	before, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	for _, tc := range []struct {
		name   string
		change func(*selection)
	}{
		{"stale", func(s *selection) { s.Checked = time.Now().Add(-91 * time.Second) }},
		{"future", func(s *selection) { s.Checked = time.Now().Add(time.Minute) }},
		{"wrong config", func(s *selection) { s.Server = "wrong" }},
		{"wrong pid", func(s *selection) { s.PID = pid + 1 }},
		{"legacy", func(s *selection) { s.PID = 0 }},
		{"RU", func(s *selection) { s.Country = "RU" }},
		{"unknown", func(s *selection) { s.Country = "ZZ" }},
		{"zero latency", func(s *selection) { s.Latency = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			tc.change(&s)
			writeJSON(c.path("selection.json"), s)
			if c.requireReady() == nil {
				t.Fatal("unsafe readiness accepted")
			}
		})
	}
	writeJSON(c.path("selection.json"), valid)
	if err := c.requireReady(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if !bytes.Equal(before, after) {
		t.Fatal("readiness did network health checks")
	}
	sameRoute(t, c, pid, cfg)
}

func TestSupervisorCorruptCacheCannotKeepRUAlive(t *testing.T) {
	c, _, pid, _ := startMonitored(t)
	if err := os.WriteFile(c.path("selection.json"), []byte("{broken-json"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := os.Getenv("DOTS_PROXY_TEST_FIXTURE")
	setHealth(t, fixture, "RU")
	// No replacement is possible: the original RU process must still be stopped.
	if err := c.saveServers([]object{startupServer(t, "blocked.test")}); err != nil {
		t.Fatal(err)
	}
	c.maintain(&monitorState{})
	if pidRunning(pid) || c.requireReady() == nil {
		t.Fatal("corrupt cache prevented stopping confirmed RU")
	}
}
func TestSupervisorStopsRUEvenWhenCacheCannotBeRead(t *testing.T) {
	c, fixture, pid, _ := startMonitored(t)
	if err := os.Mkdir(c.path("selection.json"), 0700); err != nil {
		t.Fatal(err)
	}
	setHealth(t, fixture, "RU")
	if c.maintain(&monitorState{}) == nil {
		t.Fatal("cache I/O error was hidden")
	}
	if pidRunning(pid) {
		t.Fatal("cache I/O error kept confirmed RU child alive")
	}
}
func TestSupervisorRecoversDeadWithCorruptCache(t *testing.T) {
	c, _, pid, _ := startMonitored(t)
	os.WriteFile(c.path("selection.json"), []byte("{broken-json"), 0600)
	process, _ := os.FindProcess(pid)
	process.Kill()
	process.Wait()
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal("cache corruption blocked recovery", err)
	}
	if err := c.requireReady(); err != nil {
		t.Fatal(err)
	}
}
func TestSupervisorRefusesForeignListenerWithOwnedPID(t *testing.T) {
	c, _, servers := startupFixture(t)
	// This fixture process owns the expected command/config, but no socket.
	t.Setenv("DOTS_PROXY_TEST_ROUTES", "0")
	if err := c.start(servers[0]); err != nil {
		t.Fatal(err)
	}
	pid, _ := c.readPID()
	cfg, _ := os.ReadFile(c.path("config.json"))
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port)))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "country") {
			fmt.Fprint(w, "loc=DE\n")
		} else {
			w.WriteHeader(401)
		}
	})}
	defer server.Close()
	go server.Serve(listener)
	if c.maintain(&monitorState{}) == nil {
		t.Fatal("foreign socket certified behind an owned PID")
	}
	sameRoute(t, c, pid, cfg)
	if c.requireReady() == nil {
		t.Fatal("foreign socket marked ready")
	}
}
