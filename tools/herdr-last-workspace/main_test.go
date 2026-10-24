package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type wireRequest struct {
	ID     string            `json:"id"`
	Method string            `json:"method"`
	Params map[string]string `json:"params"`
}

// Exercise the real Unix transport, persisted history and flock rather than
// mocking the application boundary or depending on a user's Herdr session.
type fixture struct {
	config
	mu          sync.Mutex
	active      string
	live        []string
	focuses     []string
	connections int
	failFocus   bool
	reply       func(wireRequest) any
}

func newFixture(t testing.TB) *fixture {
	t.Helper()
	// macOS Unix sockets have a short path limit; t.TempDir can exceed it.
	root, err := os.MkdirTemp("/tmp", "last-ws-test-")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		config: config{socketPath: filepath.Join(root, "herdr.sock"), stateDir: filepath.Join(root, "state")},
		active: "w1", live: []string{"w1", "w2", "w3"},
	}
	listener, err := net.Listen("unix", f.socketPath)
	if err != nil {
		os.RemoveAll(root)
		t.Fatal(err)
	}
	var handlers sync.WaitGroup
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.connections++
			f.mu.Unlock()
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(time.Second))
				var request wireRequest
				if json.NewDecoder(conn).Decode(&request) != nil {
					return
				}
				f.mu.Lock()
				reply := f.reply
				f.mu.Unlock()
				if reply != nil {
					json.NewEncoder(conn).Encode(reply(request))
				} else {
					json.NewEncoder(conn).Encode(f.response(request))
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		<-stopped
		handlers.Wait()
		os.RemoveAll(root)
	})
	return f
}

func (f *fixture) response(request wireRequest) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch request.Method {
	case "workspace.list":
		workspaces := make([]map[string]any, 0, len(f.live))
		for _, id := range f.live {
			workspaces = append(workspaces, map[string]any{"workspace_id": id, "focused": id == f.active})
		}
		return map[string]any{"id": request.ID, "result": map[string]any{"type": "workspace_list", "workspaces": workspaces}}
	case "workspace.focus":
		if f.failFocus {
			return map[string]any{"id": request.ID, "error": map[string]string{"code": "not_found", "message": "workspace closed"}}
		}
		f.active = request.Params["workspace_id"]
		f.focuses = append(f.focuses, f.active)
		return map[string]any{"id": request.ID, "result": map[string]any{"type": "workspace_info", "workspace": map[string]string{"workspace_id": f.active}}}
	default:
		return map[string]any{"id": request.ID, "error": map[string]string{"code": "unknown_method", "message": request.Method}}
	}
}

func (f *fixture) action(t *testing.T, action string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := run(ctx, f.config, action); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) state(t *testing.T) focusState {
	t.Helper()
	state, err := readState(f.statePath())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (f *fixture) seed(t testing.TB, state focusState) {
	t.Helper()
	if err := os.MkdirAll(f.stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeState(f.statePath(), state); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) focus(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = id
}

func TestRoundTripPreservesLegacyState(t *testing.T) {
	f := newFixture(t)
	f.seed(t, focusState{})
	if err := os.WriteFile(f.statePath(), []byte(`{"current":"w1","previous":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"workspace_focused", "workspace.focused"} {
		f.focus("w2")
		f.eventJSON = `{"event":"` + name + `","data":{"workspace_id":"w2"}}`
		f.action(t, "focused")
		if got := f.state(t); got != (focusState{"w2", "w1"}) {
			t.Fatalf("focus history = %+v", got)
		}
		f.action(t, "toggle")
		if got := f.state(t); got != (focusState{"w1", "w2"}) {
			t.Fatalf("first toggle = %+v", got)
		}
		f.action(t, "toggle")
		if got := f.state(t); got != (focusState{"w2", "w1"}) {
			t.Fatalf("second toggle = %+v", got)
		}
		// Restore w1 via an event, as if the user selected it manually.
		f.focus("w1")
		f.eventJSON = `{"event":"workspace.focused","data":{"workspace_id":"w1"}}`
		f.action(t, "focused")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.focuses, ",") != "w1,w2,w1,w2" {
		t.Fatalf("actual socket focus calls = %v", f.focuses)
	}
}

func TestPrimeAndToggleRecoverMissedFocus(t *testing.T) {
	for _, action := range []string{"prime", "toggle"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t)
			f.seed(t, focusState{"w1", "w3"})
			f.focus("w2")
			f.action(t, action)
			want := focusState{"w2", "w1"}
			if action == "toggle" {
				want = focusState{"w1", "w2"}
			}
			if got := f.state(t); got != want {
				t.Fatalf("state = %+v, want %+v", got, want)
			}
		})
	}
}

func TestFocusedIgnoresStaleAndDuplicateHooks(t *testing.T) {
	f := newFixture(t)
	f.seed(t, focusState{"w1", "w3"})
	f.focus("w2")
	f.eventJSON = `{"event":"workspace.focused","data":{"workspace_id":"w1"}}`
	f.action(t, "focused")
	if got := f.state(t); got != (focusState{"w1", "w3"}) {
		t.Fatalf("stale hook overwrote history: %+v", got)
	}
	f.eventJSON = `{"event":"workspace.focused","data":{"workspace_id":"w2"}}`
	f.action(t, "focused")
	f.action(t, "focused")
	if got := f.state(t); got != (focusState{"w2", "w1"}) {
		t.Fatalf("duplicate hook lost previous workspace: %+v", got)
	}
}

func TestNoPreviousWorkspaceIsNoOp(t *testing.T) {
	for _, previous := range []string{"", "closed", "w1"} {
		t.Run(previous, func(t *testing.T) {
			f := newFixture(t)
			f.seed(t, focusState{"w1", previous})
			f.action(t, "toggle")
			if got := f.state(t); got != (focusState{"w1", ""}) {
				t.Fatalf("state = %+v", got)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.focuses) != 0 {
				t.Fatal("attempted to focus a missing or current workspace")
			}
		})
	}
}

func TestMissingHistoryAndClosedCurrent(t *testing.T) {
	for _, current := range []string{"", "closed"} {
		t.Run(current, func(t *testing.T) {
			f := newFixture(t)
			if current != "" {
				f.seed(t, focusState{current, "w2"})
			}
			f.action(t, "prime")
			if got := f.state(t); got != (focusState{"w1", ""}) {
				t.Fatalf("state = %+v", got)
			}
		})
	}
}

func TestRejectedFocusDoesNotCommitHistory(t *testing.T) {
	f := newFixture(t)
	before := focusState{"w1", "w2"}
	f.seed(t, before)
	f.mu.Lock()
	f.failFocus = true
	f.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := run(ctx, f.config, "toggle")
	if err == nil || !strings.Contains(err.Error(), "not_found") {
		t.Fatalf("error = %v", err)
	}
	if got := f.state(t); got != before {
		t.Fatalf("failed focus changed history: %+v", got)
	}
	// A failure must also release the lock for the next invocation.
	f.mu.Lock()
	f.failFocus = false
	f.mu.Unlock()
	f.action(t, "toggle")
}

func TestConcurrentTogglesSerialize(t *testing.T) {
	f := newFixture(t)
	f.seed(t, focusState{"w1", "w2"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const count = 12
	failures := make(chan error, count)
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			failures <- run(ctx, f.config, "toggle")
		}()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := f.state(t); got != (focusState{"w1", "w2"}) {
		t.Fatalf("concurrent toggles corrupted history: %+v", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.focuses) != count {
		t.Fatalf("focus calls = %d, want %d", len(f.focuses), count)
	}
	for i, got := range f.focuses {
		want := "w2"
		if i%2 == 1 {
			want = "w1"
		}
		if got != want {
			t.Fatalf("focus call %d = %s, want %s", i, got, want)
		}
	}
}

func TestLockWaitHonorsTimeout(t *testing.T) {
	f := newFixture(t)
	f.seed(t, focusState{"w1", "w2"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lock, err := lockState(ctx, f.statePath())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	short, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	err = run(short, f.config, "toggle")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want lock timeout", err)
	}
	// The Go lock is compatible with flock used by the former Python plugin.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	f.action(t, "toggle")
}

func TestRequestIsPreparedBeforeConnecting(t *testing.T) {
	f := newFixture(t)
	client := socketClient{f.socketPath}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// A request that cannot be encoded must fail before opening a connection.
	// This also guards against encoding on an already-connected socket, which
	// can trigger Herdr 0.9.1's 100ms pending-read poll interval.
	if err := client.call(ctx, "workspace.list", make(chan int), nil); err == nil {
		t.Fatal("unencodable request accepted")
	}
	// A subsequent successful request ensures the listener has accepted any
	// earlier connection queued by the failed call before we inspect the count.
	if _, _, err := client.workspaces(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.connections != 1 {
		t.Fatalf("opened %d connections, want only the successful request", f.connections)
	}
}

func TestSocketErrorsAndTimeouts(t *testing.T) {
	cases := []struct {
		name  string
		reply func(wireRequest) any
	}{
		{"wrong ID", func(r wireRequest) any {
			return map[string]any{"id": "wrong", "result": map[string]string{"type": "workspace_list"}}
		}},
		{"missing result", func(r wireRequest) any { return map[string]any{"id": r.ID} }},
		{"server error", func(r wireRequest) any {
			return map[string]any{"id": r.ID, "error": map[string]string{"code": "busy", "message": "try later"}}
		}},
		{"wrong result type", func(r wireRequest) any {
			return map[string]any{"id": r.ID, "result": map[string]string{"type": "pong"}}
		}},
		{"read timeout", func(r wireRequest) any { time.Sleep(70 * time.Millisecond); return map[string]string{"id": r.ID} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			before := focusState{"w1", "w2"}
			f.seed(t, before)
			f.mu.Lock()
			f.reply = tc.reply
			f.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			if err := run(ctx, f.config, "toggle"); err == nil {
				t.Fatal("expected socket error")
			}
			if got := f.state(t); got != before {
				t.Fatalf("socket error changed history: %+v", got)
			}
		})
	}
}

func TestInvalidArgumentsAndEvents(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if run(ctx, f.config, "invalid") == nil {
		t.Fatal("invalid action accepted")
	}
	if run(ctx, f.config, "focused") == nil {
		t.Fatal("missing event accepted")
	}
	f.eventJSON = `{"event":"pane.focused"}`
	f.action(t, "focused")
	if _, err := os.Stat(f.stateDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unrelated hook touched state")
	}
}

func TestStateCompatibilityAndPermissions(t *testing.T) {
	f := newFixture(t)
	f.seed(t, focusState{})
	for _, data := range []string{"not JSON", `[]`, `{"current":"w1","previous":17}`, `{"current":"w1","previous":null}`} {
		if err := os.WriteFile(f.statePath(), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		f.action(t, "prime")
		if got := f.state(t); got != (focusState{"w1", ""}) {
			t.Fatalf("state from %s = %+v", data, got)
		}
	}
	entries, err := os.ReadDir(f.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".focus-") {
			t.Fatal("temporary file leaked")
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("permissions for %s = %o", entry.Name(), info.Mode().Perm())
		}
	}
}

func BenchmarkPrimeSocket(b *testing.B) {
	f := newFixture(b)
	f.seed(b, focusState{"w1", "w2"})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		err := run(ctx, f.config, "prime")
		cancel()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestConfigAndSessionNamespaces(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("HERDR_SESSION", "")
	t.Setenv("HERDR_PLUGIN_STATE_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	c, err := configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.socketPath != filepath.Join(home, ".config", "herdr", "herdr.sock") {
		t.Fatalf("default socket = %s", c.socketPath)
	}
	// Fixed expected hash matches the Python sha256(socket)[:16] namespace.
	c.socketPath = "/tmp/herdr.sock"
	if filepath.Base(c.statePath()) != "0cb0c5e4a7465217.json" {
		t.Fatalf("legacy state namespace changed: %s", c.statePath())
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("HERDR_SESSION", "named")
	c, err = configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.socketPath != filepath.Join(home, "config", "herdr", "sessions", "named", "herdr.sock") || c.stateDir != filepath.Join(home, "state", "herdr", "plugins", "dots.last-workspace") {
		t.Fatalf("XDG config = %+v", c)
	}
	namedPath := c.statePath()
	t.Setenv("HERDR_SOCKET_PATH", "/explicit/herdr.sock")
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(home, "plugin-state"))
	c, err = configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.socketPath != "/explicit/herdr.sock" || c.stateDir != filepath.Join(home, "plugin-state") || c.statePath() == namedPath {
		t.Fatalf("plugin overrides = %+v", c)
	}
}
