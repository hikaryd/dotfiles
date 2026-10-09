package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func metricFixture(changes object) string {
	m := object{"http_code": 200, "size_download": 1000, "speed_download": 2000000, "time_starttransfer": 0.02, "time_total": 0.5}
	for k, v := range changes {
		m[k] = v
	}
	return string(jsonBytes(m))
}
func TestMeasurementHTTPAndPartialFailure(t *testing.T) {
	fakeCommands(t)
	for _, c := range []struct {
		metrics object
		why     string
	}{{object{"http_code": 403}, "unexpected_http"}, {object{"http_code": 302}, "unexpected_http"}, {object{"size_download": 999}, "incomplete_download"}} {
		t.Setenv("VPN_TEST_CURL_METRICS", metricFixture(c.metrics))
		r := measurement(context.Background(), 1234, "fake-password", 1000, time.Second, defaultURL)
		equal(t, r["ok"], false)
		equal(t, r["mbps"], nil)
		equal(t, r["error"], c.why)
	}
}
func TestMeasurementTransportAndInvalidMetrics(t *testing.T) {
	fakeCommands(t)
	for _, c := range []struct{ metrics, exit string }{{metricFixture(nil), "28"}, {`{"speed_download":NaN}`, "0"}, {metricFixture(object{"time_total": -1}), "0"}, {"{}", "0"}, {"not JSON", "0"}, {metricFixture(object{"http_code": true}), "0"}} {
		t.Setenv("VPN_TEST_CURL_METRICS", c.metrics)
		t.Setenv("VPN_TEST_CURL_EXIT", c.exit)
		r := measurement(context.Background(), 1234, "fake-password", 1000, time.Second, defaultURL)
		equal(t, r["ok"], false)
		equal(t, r["mbps"], nil)
	}
}
func TestMeasurementPasswordStdinAndURL(t *testing.T) {
	fakeCommands(t)
	r := measurement(context.Background(), 1234, "fake-password", 1000, time.Second, "https://other.example/file?size={bytes}")
	equal(t, r["ok"], true)
	equal(t, r["mbps"], float64(16))
	rows := traces(t)
	args := rows[0]["args"].([]any)
	equal(t, args[0], "--disable")
	if strings.Contains(string(jsonBytes(args)), "fake-password") {
		t.Fatal("password in argv")
	}
	if !strings.Contains(str(rows[0]["stdin"]), "fake-password") {
		t.Fatal("missing stdin password")
	}
	if !strings.Contains(str(rows[0]["stdin"]), "https://other.example/file?size=1000") {
		t.Fatal(args)
	}
}
func TestMeasurementOverallDeadline(t *testing.T) {
	fakeCommands(t)
	t.Setenv("VPN_TEST_CURL_SLEEP", "3s")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	begin := time.Now()
	r := measurement(ctx, 1234, "fake-password", 1000, 20*time.Second, defaultURL)
	equal(t, r["ok"], false)
	if time.Since(begin) > time.Second {
		t.Fatal("deadline not enforced")
	}
	for _, row := range traces(t) {
		pid := int(row["pid"].(float64))
		if syscall.Kill(pid, 0) == nil {
			t.Fatal("curl child remains")
		}
	}
}
func TestUnsafeBenchmarkURLsRedacted(t *testing.T) {
	for _, u := range []string{"https://alice:fake-secret@example.com/file", "file:///etc/passwd", "https://example.com:99999/file", "https://example.com/file\nheader", "https://example.com/{unknown}", "https://example.com/#fragment"} {
		_, e := downloadURL(u, 1000)
		if e == nil || strings.Contains(e.Error(), "fake-secret") {
			t.Fatal(e)
		}
	}
}
func TestOnlyManualWithoutAuto(t *testing.T) {
	cfg := object{"outbounds": []object{{"tag": "manual", "type": "vless", "uuid": "fake-uuid"}}}
	n, e := selectCandidates(cfg, []string{"manual"}, nil, false)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, n[0]["tag"], "VPS-direct")
	equal(t, n[1]["tag"], "manual")
	if _, e = selectCandidates(cfg, []string{"missing"}, nil, false); e == nil {
		t.Fatal("accepted missing")
	}
}
func TestAllProxyProtocolsAndHelpers(t *testing.T) {
	nodes := []object{}
	for i, p := range proxyTypes {
		nodes = append(nodes, object{"type": p, "tag": p, "password": "fake-secret"})
		if i == 0 {
			nodes = append(nodes, object{"type": p, "tag": "second-socks"})
		}
	}
	proxies := append([]object{}, nodes...)
	for _, p := range []string{"direct", "block", "dns", "bridge", "selector", "urltest"} {
		nodes = append(nodes, object{"type": p, "tag": p})
	}
	n, e := selectCandidates(object{"outbounds": nodes}, nil, nil, true)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, len(n), len(proxies)+1)
	for _, p := range proxies {
		n, e := selectCandidates(object{"outbounds": nodes}, []string{str(p["tag"])}, nil, false)
		if e != nil {
			t.Fatal(e)
		}
		equal(t, n[1]["tag"], p["tag"])
	}
}
func TestDefaultAutoPoolRetained(t *testing.T) {
	cfg := object{"outbounds": []object{{"type": "hysteria2", "tag": "finland"}, {"type": "trojan", "tag": "moscow"}, {"type": "urltest", "tag": "auto", "outbounds": []string{"finland"}}}}
	n, e := selectCandidates(cfg, nil, nil, false)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, len(n), 2)
	equal(t, n[1]["tag"], "finland")
}
func eventRows(t *testing.T, b *bytes.Buffer) []object {
	t.Helper()
	rows := []object{}
	for _, l := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		var o object
		if json.Unmarshal([]byte(l), &o) != nil {
			t.Fatalf("bad JSON: %s", l)
		}
		rows = append(rows, o)
	}
	return rows
}
func TestScreeningDefaultsAndSkippedBudget(t *testing.T) {
	fakeCommands(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, jsonBytes(object{"outbounds": []object{{"type": "hysteria2", "tag": "finland", "password": "fake-secret"}, {"type": "trojan", "tag": "moscow", "password": "fake-secret"}}}), 0600)
	var out bytes.Buffer
	status := benchmarkMain(context.Background(), []string{"--config", cfg, "--all", "--max-duration", "0.000000001"}, &out, io.Discard)
	equal(t, status, 1)
	rows := eventRows(t, &out)
	equal(t, rows[0]["samples"], float64(1))
	equal(t, rows[0]["bytes"], float64(1000000))
	equal(t, rows[0]["timeout"], float64(10))
	equal(t, rows[0]["total_trials"], float64(3))
	equal(t, rows[len(rows)-1]["skipped"], float64(3))
	if strings.Contains(out.String(), "fake-secret") {
		t.Fatal("leak")
	}
}
func TestBenchmarkSourcesAndNonzeroFailures(t *testing.T) {
	requireSocket(t)
	fakeCommands(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, jsonBytes(object{"outbounds": []object{{"type": "vless", "tag": "manual", "uuid": "fake-uuid"}}}), 0600)
	var out bytes.Buffer
	status := benchmarkMain(context.Background(), []string{"--config", cfg, "--only", "manual", "--samples", "1", "--bytes", "1000", "--url", defaultURL, "--url", "https://failure.example/file?token=fake-secret&bytes={bytes}"}, &out, io.Discard)
	equal(t, status, 1)
	rows := eventRows(t, &out)
	count := 0
	for _, r := range rows {
		if r["event"] == "summary" {
			count++
			if r["source"] == "2:failure.example" {
				equal(t, r["median_mbps"], nil)
			}
		}
	}
	equal(t, count, 4)
	if strings.Contains(out.String(), "fake-secret") || strings.Contains(out.String(), "fake-uuid") {
		t.Fatal("leak")
	}
	assertCleanChildren(t)
}
func assertCleanChildren(t *testing.T) {
	t.Helper()
	for _, r := range traces(t) {
		if path := str(r["path"]); path != "" {
			if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatalf("private config remains: %v", e)
			}
		}
		if r["command"] == "sing-box" || r["command"] == "xray" {
			equal(t, r["mode"], float64(0600))
			equal(t, r["dir_mode"], float64(0700))
			if syscall.Kill(int(r["pid"].(float64)), 0) == nil {
				t.Fatal("child remains alive")
			}
		}
	}
}
func TestRealBenchmarkCleanupOnFailureAndCancel(t *testing.T) {
	requireSocket(t)
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "cancel"}[cancelled], func(t *testing.T) {
			fakeCommands(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				t.Setenv("VPN_TEST_CURL_SLEEP", "10s")
				go func() { time.Sleep(200 * time.Millisecond); cancel() }()
			} else {
				t.Setenv("VPN_TEST_CURL_EXIT", "28")
			}
			r, e := benchmarkNode(ctx, object{"type": "direct", "tag": "direct"}, 1000, time.Second, defaultURL)
			if e == nil && r["ok"] == true {
				t.Fatal("unexpected success")
			}
			assertCleanChildren(t)
		})
	}
}
func TestIgnoringTermChildKilledAndReaped(t *testing.T) {
	dir := fakeCommands(t)
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("VPN_TEST_READY", ready)
	p, e := startChild(exec.Command(filepath.Join(dir, "stubborn")))
	if e != nil {
		t.Fatal(e)
	}
	defer p.stop(50 * time.Millisecond)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, e = os.Stat(ready); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper startup timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.stop(50 * time.Millisecond)
	if syscall.Kill(p.cmd.Process.Pid, 0) == nil {
		t.Fatal("owned child still exists")
	}
	if p.cmd.Process.Pid == os.Getpid() {
		t.Fatal("parent targeted")
	}
}
