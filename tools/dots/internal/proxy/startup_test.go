package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A real child process listening as an HTTP proxy, without external networking.
// One route works immediately; the others stall until their request is canceled.
func serveFixtureRoute(fixture string) {
	data, _ := os.ReadFile(os.Args[3])
	var cfg struct {
		Inbounds  []struct{ Port int }
		Outbounds []struct {
			Settings struct{ Vnext []struct{ Address string } }
		}
	}
	if json.Unmarshal(data, &cfg) != nil || len(cfg.Inbounds) == 0 || len(cfg.Outbounds) == 0 || len(cfg.Outbounds[0].Settings.Vnext) == 0 {
		os.Exit(2)
	}
	address := cfg.Outbounds[0].Settings.Vnext[0].Address
	probe := strings.Contains(os.Args[3], ".probe-")
	if !probe {
		args, _ := json.Marshal(os.Args)
		_ = os.WriteFile(filepath.Join(fixture, "xray-argv"), args, 0600)
		_ = os.WriteFile(filepath.Join(fixture, "child-pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(cfg.Inbounds[0].Port))
	if err != nil {
		os.Exit(3)
	}
	_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.OpenFile(filepath.Join(fixture, "requests"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		fmt.Fprintf(f, "%s %t %s\n", address, probe, r.URL.Path)
		f.Close()
		if r.URL.Path == "/stream" {
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			fmt.Fprint(rw, "HTTP/1.1 200 OK\r\n\r\n")
			rw.Flush()
			scanner := bufio.NewScanner(rw)
			for scanner.Scan() {
				fmt.Fprintln(rw, scanner.Text())
				rw.Flush()
			}
			return
		}
		if !probe {
			health, _ := os.ReadFile(filepath.Join(fixture, "health"))
			switch string(health) {
			case "fail":
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			case "RU":
				fmt.Fprintln(w, "loc=RU")
				return
			}
		}
		if strings.HasPrefix(address, "slow-") {
			<-r.Context().Done()
			return
		}
		if address == "second.test" {
			time.Sleep(30 * time.Millisecond)
		}
		if r.URL.Path == "/country" {
			country := "DE"
			if address == "ru.test" || (address == "bad-live.test" && !probe) {
				country = "RU"
			} else if address == "unknown.test" {
				country = "ZZ"
			}
			fmt.Fprintln(w, "loc="+country)
		} else if address == "blocked.test" {
			w.WriteHeader(http.StatusForbidden)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
}

func startupFixture(t *testing.T) (*controller, string, []object) {
	t.Helper()
	fixture := fakeCommands(t)
	t.Setenv("DOTS_PROXY_TEST_ROUTES", "1")
	c := testController(t)
	c.countryTarget, c.apiTarget = "http://country.test/country", "http://openai.test/models"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.port = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	cleanupChild(t, c, fixture)
	servers := make([]object, 75)
	for i := range servers {
		address := fmt.Sprintf("slow-%d.test", i)
		if i == 0 {
			address = "fast.test"
		}
		servers[i] = startupServer(t, address)
	}
	if err := c.saveServers(servers); err != nil {
		t.Fatal(err)
	}
	return c, fixture, servers
}

func startupServer(t *testing.T, address string) object {
	t.Helper()
	server, err := parseVLESS("vless://fixture-id@" + address + ":443#Fixture")
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func TestAutoStartupDoesNotWaitForSlowRoutes(t *testing.T) {
	c, fixture, servers := startupFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c.ctx = ctx
	started := time.Now()
	if err := c.autoStart(false); err != nil {
		t.Fatalf("startup waited for slow routes despite a verified fast route (%s): %v", time.Since(started).Round(time.Millisecond), err)
	}
	if !c.matchesRunning(servers[0]) {
		t.Fatal("verified route was not activated")
	}
	requests, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if strings.Count(string(requests), "fast.test true /models\n") != 1 || strings.Count(string(requests), "fast.test false /models\n") != 1 {
		t.Fatalf("startup should check OpenAI once per listener, got:\n%s", requests)
	}
	dirs, _ := filepath.Glob(c.path(".probe-*"))
	if len(dirs) != 0 {
		t.Fatal("returned before canceled probes were reaped", dirs)
	}
}

func TestAutoStartupRechecksExpiredSelection(t *testing.T) {
	c, fixture, servers := startupFixture(t)
	if err := c.start(servers[0]); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, len(servers))
	for i, server := range servers {
		keys[i] = c.serverKey(server)
	}
	cached := selection{Version: 1, Servers: digest(keys), Server: keys[0], Checked: time.Now().Add(-24 * time.Hour), measurement: measurement{"DE", time.Millisecond}}
	if err := writeJSON(c.path("selection.json"), cached); err != nil {
		t.Fatal(err)
	}
	// A subscription update/reorder must not discard a still-present route.
	if err := c.saveServers(append(servers[1:], servers[0])); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c.ctx = ctx
	if err := c.autoStart(false); err != nil {
		t.Fatalf("expired cache triggered a full scan instead of rechecking the running route: %v", err)
	}
	data, _ := os.ReadFile(c.path("selection.json"))
	var refreshed selection
	if json.Unmarshal(data, &refreshed) != nil || !refreshed.Checked.After(cached.Checked) {
		t.Fatal("live verification did not refresh the cache")
	}
	requests, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if got := string(requests); got != "fast.test false /country\nfast.test false /models\n" {
		t.Fatalf("expected only a fresh live country/OpenAI check, got:\n%s", got)
	}
}

func TestAutoStartupRestartsExpiredSelectionWithoutFullScan(t *testing.T) {
	c, fixture, servers := startupFixture(t)
	keys := make([]string, len(servers))
	for i, server := range servers {
		keys[i] = c.serverKey(server)
	}
	if err := writeJSON(c.path("selection.json"), selection{Version: 1, Server: keys[0], Checked: time.Now().Add(-24 * time.Hour), measurement: measurement{"DE", time.Millisecond}}); err != nil {
		t.Fatal(err)
	}
	if err := c.autoStart(false); err != nil {
		t.Fatal(err)
	}
	requests, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	want := "fast.test true /country\nfast.test true /models\nfast.test false /country\nfast.test false /models\n"
	if string(requests) != want {
		t.Fatalf("stopped cached route should get one isolated and one live check, got:\n%s", requests)
	}
}

func TestAutoStartupRejectsUnsafeRoutes(t *testing.T) {
	c, fixture, _ := startupFixture(t)
	servers := []object{startupServer(t, "ru.test"), startupServer(t, "unknown.test"), startupServer(t, "blocked.test")}
	if err := c.saveServers(servers); err != nil {
		t.Fatal(err)
	}
	if err := c.autoStart(false); err == nil || !strings.Contains(err.Error(), "direct/RU fallback is disabled") {
		t.Fatalf("unsafe route accepted: %v", err)
	}
	for _, name := range []string{"xray.pid", "selection.json", "config.json"} {
		if _, err := os.Stat(c.path(name)); !os.IsNotExist(err) {
			t.Fatal("unsafe route changed live state", name)
		}
	}
	requests, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if strings.Contains(string(requests), "ru.test true /models") || strings.Contains(string(requests), "unknown.test true /models") {
		t.Fatal("OpenAI was contacted despite a rejected exit country")
	}
}

func TestAutoStartupFallsBackAfterFinalCheckFails(t *testing.T) {
	c, fixture, _ := startupFixture(t)
	servers := []object{startupServer(t, "bad-live.test"), startupServer(t, "second.test")}
	if err := c.saveServers(servers); err != nil {
		t.Fatal(err)
	}
	if err := c.autoStart(false); err != nil {
		t.Fatal(err)
	}
	if !c.matchesRunning(servers[1]) {
		t.Fatal("did not fall back after the selected listener had a RU exit")
	}
	requests, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if !strings.Contains(string(requests), "bad-live.test false /country") || strings.Contains(string(requests), "bad-live.test false /models") {
		t.Fatal("final country verification was skipped or ignored")
	}
}

func TestAutoForcedSelectionStillBenchmarksEveryRoute(t *testing.T) {
	c, fixture, _ := startupFixture(t)
	servers := []object{startupServer(t, "fast.test"), startupServer(t, "second.test")}
	if err := c.saveServers(servers); err != nil {
		t.Fatal(err)
	}
	if err := c.autoStart(true); err != nil {
		t.Fatal(err)
	}
	if !c.matchesRunning(servers[0]) {
		t.Fatal("full benchmark did not select the fastest route")
	}
	requests, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	for _, address := range []string{"fast.test", "second.test"} {
		if strings.Count(string(requests), address+" true /models\n") != 3 {
			t.Fatalf("--auto should benchmark every route with three samples, got:\n%s", requests)
		}
	}
}
