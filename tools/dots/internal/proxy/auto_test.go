package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBenchmarkChoosesFastestVerifiedExit(t *testing.T) {
	servers := []object{{"name": "RU fastest"}, {"name": "DE"}, {"name": "NL"}, {"name": "unknown"}, {"name": "blocked"}}
	results := benchmark(context.Background(), servers, 4, func(_ context.Context, s object) (measurement, error) {
		switch s["name"] {
		case "RU fastest":
			return measurement{"RU", time.Millisecond}, nil
		case "DE":
			return measurement{"DE", 30 * time.Millisecond}, nil
		case "NL":
			return measurement{"NL", 15 * time.Millisecond}, nil
		case "unknown":
			return measurement{"ZZ", time.Millisecond}, nil
		default:
			return measurement{}, errors.New("HTTP 403")
		}
	})
	if results[0].index != 2 || results[0].err != nil || results[1].index != 1 {
		t.Fatalf("wrong ranking: %+v", results)
	}
	for _, result := range results[2:] {
		if result.err == nil {
			t.Fatal("unverified/unavailable route accepted")
		}
	}
}

func TestBenchmarkBoundedParallelismAndCancellation(t *testing.T) {
	servers := make([]object, 20)
	var active, peak atomic.Int32
	results := benchmark(context.Background(), servers, 4, func(context.Context, object) (measurement, error) {
		n := active.Add(1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return measurement{"DE", time.Millisecond}, nil
	})
	if peak.Load() > 4 || peak.Load() < 2 || len(results) != 20 {
		t.Fatalf("parallelism %d, results %d", peak.Load(), len(results))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results = benchmark(ctx, servers, 4, func(context.Context, object) (measurement, error) {
		t.Error("probe started after cancellation")
		return measurement{}, nil
	})
	if len(results) != 0 {
		t.Fatal("results returned after cancellation", results)
	}
}

func TestSelectionCacheRetainsHintsButRejectsInvalidRoutes(t *testing.T) {
	now := time.Now()
	keys := []string{"a", "b"}
	good := selection{Version: 1, Servers: digest(keys), Server: "b", Checked: now.Add(-24 * time.Hour), measurement: measurement{"DE", time.Millisecond}}
	if cachedSelection(good, keys, now) != 1 {
		t.Fatal("old selection not retained for a live recheck")
	}
	if cachedSelection(good, []string{"b", "new"}, now) != 0 {
		t.Fatal("changes to other routes invalidated the selected route")
	}
	for _, change := range []func(*selection){
		func(s *selection) { s.Version = 0 },
		func(s *selection) { s.Server = "removed" },
		func(s *selection) { s.Checked = now.Add(time.Minute) },
		func(s *selection) { s.Country = "RU" },
		func(s *selection) { s.Country = "" },
		func(s *selection) { s.Latency = 0 },
	} {
		bad := good
		change(&bad)
		if cachedSelection(bad, keys, now) != -1 {
			t.Fatalf("invalid cache accepted: %+v", bad)
		}
	}
	if medianLatency([]time.Duration{90, 10, 20}) != 20 {
		t.Fatal("not median")
	}
}

func TestCandidatesDeduplicateConfigAndRepairOldFragments(t *testing.T) {
	c := testController(t)
	one := fixtureVLESS(t)
	two := fixtureVLESS(t)
	two["name"] = "another label"
	two["raw_url"] = strings.Split(one["raw_url"].(string), "#")[0] + "#another label"
	three, err := parseVLESS("vless://fixture-id@other.test:443?type=xhttp#DE #2")
	if err != nil {
		t.Fatal(err)
	}
	three["type"] = "xhttp#old-parser-bug"
	if err := c.saveServers([]object{one, two, three, {"invalid": true}}); err != nil {
		t.Fatal(err)
	}
	servers, err := c.candidates()
	if err != nil || len(servers) != 2 || servers[1]["type"] != "xhttp" || servers[1]["name"] != "DE #2" {
		t.Fatalf("candidates: %+v, %v", servers, err)
	}
	if err := writeJSON(c.path("selection.json"), selection{Server: c.serverKey(one)}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(c.path("selection.json"))
	if bytes.Contains(data, []byte("fixture-id")) || bytes.Contains(data, []byte("vless://")) {
		t.Fatal("credentials in selection cache")
	}
	info, _ := os.Stat(c.path("selection.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("selection cache not private")
	}
}

func TestAutoFirstRunNeverConsumesStdin(t *testing.T) {
	c := testController(t)
	if _, err := c.candidates(); err == nil {
		t.Fatal("first run should require explicit setup")
	}
	// The helper above fails before accessing stdin or looking for xray.
	if strings.Contains(c.out.(*bytes.Buffer).String(), "URL") {
		t.Fatal("autostart prompted")
	}
}

func TestAutoFailsClosedWithoutChangingLiveConfig(t *testing.T) {
	c := testController(t)
	t.Setenv("PATH", t.TempDir())
	if err := c.saveServers([]object{fixtureVLESS(t)}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path("config.json"), []byte("keep live config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.autoStart(false); err == nil || !strings.Contains(err.Error(), "direct/RU fallback is disabled") {
		t.Fatal("autostart did not fail closed", err)
	}
	data, _ := os.ReadFile(c.path("config.json"))
	if string(data) != "keep live config" {
		t.Fatal("failed benchmark changed live config")
	}
	for _, name := range []string{"xray.pid", "selection.json"} {
		if _, err := os.Stat(c.path(name)); !os.IsNotExist(err) {
			t.Fatal("state written after failed benchmark", name)
		}
	}
	if strings.Contains(c.out.(*bytes.Buffer).String(), "fixture-id") {
		t.Fatal("credentials in diagnostics")
	}
}

func TestProbeFailureWithholdsSecretsAndRemovesTemporaryConfig(t *testing.T) {
	fakeCommands(t)
	c := testController(t)
	t.Setenv("DOTS_PROXY_TEST_XRAY_FAIL", "1")
	_, err := c.withProbe(context.Background(), fixtureVLESS(t), func(context.Context, string) (measurement, error) {
		t.Error("check ran with a failed xray")
		return measurement{}, nil
	})
	if err == nil || strings.Contains(err.Error(), "fixture-password-do-not-log") {
		t.Fatal("bad failure diagnostics", err)
	}
	dirs, _ := filepath.Glob(c.path(".probe-*"))
	if len(dirs) != 0 {
		t.Fatal("probe config retained after failure", dirs)
	}
}

func TestMeasureRouteUsesProxyAndRejectsBlockedOrUnknownExits(t *testing.T) {
	var country atomic.Value
	country.Store("DE")
	var status atomic.Int32
	status.Store(401)
	var countryCalls, apiCalls atomic.Int32
	origin := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/country" {
			countryCalls.Add(1)
			fmt.Fprintf(w, "ip=192.0.2.1\nloc=%s\n", country.Load())
			return
		}
		apiCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("credentials sent by probe")
		}
		w.WriteHeader(int(status.Load()))
	}))
	target, _ := url.Parse(origin.URL)
	forward := httputil.NewSingleHostReverseProxy(target)
	var proxyCalls atomic.Int32
	proxy := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		if !r.URL.IsAbs() {
			t.Error("not an explicit forward proxy request")
		}
		forward.ServeHTTP(w, r)
	}))
	t.Setenv("NO_PROXY", "*")
	measure := func() (measurement, error) {
		return measureRoute(context.Background(), proxy.URL, origin.URL+"/country", origin.URL+"/models", 3)
	}
	m, err := measure()
	if err != nil || m.Country != "DE" || m.Latency <= 0 || proxyCalls.Load() != 4 || apiCalls.Load() != 3 {
		t.Fatalf("measurement %+v, %v; proxy calls %d", m, err, proxyCalls.Load())
	}
	for _, code := range []int{200, 401, 301, 403, 429, 500, 503} {
		status.Store(int32(code))
		_, err := measure()
		if (err == nil) != (code == 200 || code == 401) {
			t.Fatalf("HTTP %d: %v", code, err)
		}
	}
	for _, code := range []string{"RU", "", "XX", "ZZ", "T1", "de", "RUS"} {
		country.Store(code)
		before := apiCalls.Load()
		if _, err := measure(); err == nil || apiCalls.Load() != before {
			t.Fatalf("country %q accepted or benchmarked", code)
		}
	}
}

func TestProbeIsolationAndCleanup(t *testing.T) {
	fakeCommands(t)
	c := testController(t)
	if err := os.WriteFile(c.path("config.json"), []byte("live config untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	var probeURL string
	m, err := c.withProbe(context.Background(), fixtureVLESS(t), func(ctx context.Context, proxy string) (measurement, error) {
		probeURL = proxy
		resp, err := http.Get(proxy)
		if err != nil {
			return measurement{}, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "isolated probe" {
			t.Fatal(string(body))
		}
		return measurement{"DE", time.Millisecond}, nil
	})
	if err != nil || m.Country != "DE" || probeURL == c.proxyURL() {
		t.Fatalf("probe %+v %v %s", m, err, probeURL)
	}
	data, _ := os.ReadFile(c.path("config.json"))
	if string(data) != "live config untouched" {
		t.Fatal("benchmark changed live config")
	}
	dirs, _ := filepath.Glob(c.path(".probe-*"))
	if len(dirs) != 0 {
		t.Fatal("probe config retained", dirs)
	}
	client := &http.Client{Timeout: 100 * time.Millisecond}
	if resp, err := client.Get(probeURL); err == nil {
		resp.Body.Close()
		t.Fatal("probe process retained")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.withProbe(ctx, fixtureVLESS(t), func(context.Context, string) (measurement, error) {
		t.Error("check ran after cancellation")
		return measurement{}, nil
	}); err == nil {
		t.Fatal("cancellation ignored")
	}
}
