package vpn

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func xNode(t *testing.T, changes map[string]string) (*url.URL, map[string]string) {
	t.Helper()
	q := url.Values{"type": {"xhttp"}, "security": {"reality"}, "encryption": {"none"}, "flow": {""}, "fp": {"chrome"}, "sni": {"cover.example"}, "pbk": {"fake-key"}}
	for k, v := range changes {
		q.Set(k, v)
	}
	u, m, e := parseXHTTP("vless://fake-uuid@node.example:443?" + q.Encode())
	if e != nil {
		t.Fatal(e)
	}
	return u, m
}
func TestXHTTPSourcesFixedAndTemplate(t *testing.T) {
	u, e := validatedURLs([]string{"https://speed.example/file?bytes={bytes}", "https://other.example/10Mb.dat"}, 1000)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, u, []string{"https://speed.example/file?bytes=1000", "https://other.example/10Mb.dat"})
}
func TestXHTTPInvalidSourcesRedacted(t *testing.T) {
	for _, urls := range [][]string{{"https://user:fake-secret@speed.example/file"}, {"https://speed.example/file\n"}, {"https://speed.example/file?size={unknown}"}, {"http://speed.example/file"}, {"https://speed.example/file", "https://speed.example/file", "https://speed.example/file", "https://speed.example/file", "https://speed.example/file"}} {
		_, e := validatedURLs(urls, 1000)
		if e == nil || strings.Contains(e.Error(), "fake-secret") {
			t.Fatal(e)
		}
	}
}
func TestXHTTPRealityOptionalFields(t *testing.T) {
	u, q := xNode(t, nil)
	n, e := buildXHTTP(u, q)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, obj(n["settings"])["flow"], "")
	s := obj(n["streamSettings"])
	equal(t, s["network"], "xhttp")
	equal(t, s["xhttpSettings"], object{"mode": "auto", "path": "/"})
	equal(t, obj(s["realitySettings"])["password"], "fake-key")
	equal(t, obj(s["realitySettings"])["shortId"], "")
}
func TestXHTTPDecodeOnceNestedExtra(t *testing.T) {
	extra := object{"xmux": object{"maxConnections": float64(3)}, "downloadSettings": object{"address": "down.example", "port": float64(443), "network": "xhttp", "security": "reality"}}
	u, q := xNode(t, map[string]string{"path": "/literal%2F", "extra": string(jsonBytes(extra))})
	n, e := buildXHTTP(u, q)
	if e != nil {
		t.Fatal(e)
	}
	x := obj(obj(n["streamSettings"])["xhttpSettings"])
	equal(t, x["path"], "/literal%2F")
	equal(t, x["extra"], extra)
}
func TestXHTTPVisionAndExtraErrors(t *testing.T) {
	for _, c := range []struct {
		q   map[string]string
		why string
	}{{map[string]string{"flow": "xtls-rprx-vision"}, "vision_incompatible_with_xhttp_none"}, {map[string]string{"extra": "[]"}, "xhttp_extra_not_object"}, {map[string]string{"extra": "fake-secret"}, "invalid_xhttp_extra_json"}, {map[string]string{"flow": "bad"}, "unsupported_vless_flow"}, {map[string]string{"encryption": ""}, "empty_vless_encryption"}, {map[string]string{"mode": "bad"}, "invalid_xhttp_mode"}, {map[string]string{"path": ""}, "empty_xhttp_path"}, {map[string]string{"host": ""}, "empty_xhttp_host"}, {map[string]string{"pbk": ""}, "missing_reality_password_or_fingerprint"}, {map[string]string{"sni": ""}, "empty_reality_server_name"}, {map[string]string{"security": "bad"}, "unsupported_transport_security"}} {
		u, q := xNode(t, c.q)
		_, e := buildXHTTP(u, q)
		if e == nil {
			t.Fatal(c.why)
		}
		equal(t, e.Error(), c.why)
	}
}
func TestXHTTPFetchErrorRedacted(t *testing.T) {
	_, e := readURL(context.Background(), http.DefaultClient, "https://fake-token@private.example/%xx", 1000, "test")
	if e == nil {
		t.Fatal("invalid URI accepted")
	}
	equal(t, e.Error(), "source_fetch_failed")
}
func TestXHTTPOnlyComplete200HasThroughput(t *testing.T) {
	fakeCommands(t)
	for i, c := range []struct{ output, exit string }{{"302\t1000\t0.5\t2000", "0"}, {"200\t999\t0.5\t2000", "0"}, {"200\t1000\t0.5\t2000", "28"}, {"200\t1000\tNaN\t2000", "0"}, {"200\t1000\t0.5\t-1", "0"}, {"invalid", "0"}, {"200\t1000\t0.5\t2000", "0"}} {
		t.Setenv("VPN_TEST_CURL_METRICS", c.output)
		t.Setenv("VPN_TEST_CURL_EXIT", c.exit)
		r, e := xhttpTrial(context.Background(), 12345, [2]string{"fake-user", "fake-password"}, "https://speed.example/file", xhttpArgs{size: 1000, timeout: 2}, t.TempDir(), i, 1)
		if e != nil {
			t.Fatal(e)
		}
		equal(t, r["success"], i == 6)
		if i == 6 {
			equal(t, r["mbps"], 0.016)
		} else {
			equal(t, r["mbps"], nil)
		}
	}
}
func TestXHTTPCurlStdinRemoteDNSDeadline(t *testing.T) {
	fakeCommands(t)
	t.Setenv("VPN_TEST_CURL_SLEEP", "3s")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	begin := time.Now()
	r, e := xhttpTrial(ctx, 12345, [2]string{"fake-user", "fake-password"}, "https://speed.example/file", xhttpArgs{size: 1000, timeout: 2}, t.TempDir(), 1, 1)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, r["success"], false)
	if time.Since(begin) > time.Second {
		t.Fatal("deadline escaped")
	}
	rows := traces(t)
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	a := rows[0]["args"].([]any)
	equal(t, a[0], "--disable")
	equal(t, a[slices.Index(a, any("--noproxy"))+1], "")
	equal(t, a[slices.Index(a, any("--socks5-hostname"))+1], "127.0.0.1:12345")
	if strings.Contains(string(jsonBytes(a)), "fake-password") {
		t.Fatal("argv leak")
	}
	if !strings.Contains(str(rows[0]["stdin"]), "fake-password") {
		t.Fatal("stdin missing")
	}
}
func TestXHTTPGroupStoppedWithoutParent(t *testing.T) {
	dir := fakeCommands(t)
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("VPN_TEST_READY", ready)
	p, e := startChild(exec.Command(filepath.Join(dir, "stubborn")))
	if e != nil {
		t.Fatal(e)
	}
	p.stop(50 * time.Millisecond)
	if syscall.Kill(p.cmd.Process.Pid, 0) == nil {
		t.Fatal("child survives")
	}
	if syscall.Kill(os.Getpid(), 0) != nil {
		t.Fatal("parent affected")
	}
}
func TestXHTTPCheckPrivateConfigNoLaunch(t *testing.T) {
	requireSocket(t)
	dir := fakeCommands(t)
	u, q := xNode(t, nil)
	var out bytes.Buffer
	code, e := xhttpSession(context.Background(), &out, xhttpArgs{index: 6, size: 1000, samples: 1, timeout: 2, duration: 10, check: true}, filepath.Join(dir, "xray"), u, q, []string{defaultURL})
	if e != nil {
		t.Fatal(e)
	}
	equal(t, code, 0)
	rows := traces(t)
	equal(t, len(rows), 1)
	a := rows[0]["args"].([]any)
	if !slices.Contains(a, any("-test")) {
		t.Fatal(a)
	}
	cfg := obj(rows[0]["config"])
	in := obj(cfg["inbounds"].([]any)[0])
	equal(t, in["listen"], "127.0.0.1")
	equal(t, obj(in["settings"])["auth"], "password")
	equal(t, len(cfg["outbounds"].([]any)), 1)
	assertCleanChildren(t)
	if strings.Contains(out.String(), "fake") {
		t.Fatal("credential output")
	}
	equal(t, eventRows(t, &out)[0]["handshake_tested"], false)
}
func TestXHTTPStartFailureRemovesConfigLogs(t *testing.T) {
	requireSocket(t)
	dir := fakeCommands(t)
	t.Setenv("VPN_TEST_START_FAIL", "1")
	u, q := xNode(t, nil)
	_, e := xhttpSession(context.Background(), io.Discard, xhttpArgs{index: 6, size: 1000, samples: 1, timeout: 2, duration: 10}, filepath.Join(dir, "xray"), u, q, []string{defaultURL})
	if e == nil || e.Error() != "xray_exited_before_listening" {
		t.Fatal(e)
	}
	assertCleanChildren(t)
	entries, _ := os.ReadDir(os.TempDir())
	equal(t, len(entries), 0)
}
func TestXHTTPTLSAndEncryptedVision(t *testing.T) {
	u, q := xNode(t, map[string]string{"security": "tls", "encryption": "fixture-encryption", "flow": "xtls-rprx-vision", "alpn": "h2,http/1.1", "ech": "fake-ech", "pcs": "fake-pcs", "vcn": "cover.example"})
	n, e := buildXHTTP(u, q)
	if e != nil {
		t.Fatal(e)
	}
	tls := obj(obj(n["streamSettings"])["tlsSettings"])
	equal(t, tls["alpn"], []string{"h2", "http/1.1"})
	equal(t, tls["echConfigList"], "fake-ech")
	equal(t, tls["pinnedPeerCertSha256"], "fake-pcs")
	equal(t, tls["verifyPeerCertByName"], "cover.example")
}
func TestXHTTPAuthorityAndDuplicates(t *testing.T) {
	for _, s := range []string{"vless://id@host:443?type=xhttp&type=xhttp", "trojan://id@host:443?type=xhttp", "vless://id:password@host:443?type=xhttp", "vless://@host:443?type=xhttp", "vless://id@host?type=xhttp", "vless://id@host:99999?type=xhttp"} {
		if _, _, e := parseXHTTP(s); e == nil {
			t.Fatal("invalid source accepted")
		}
	}
	u, _, e := parseXHTTP("vless://fake%252Fuuid@[2001:db8::1]:443?type=splithttp")
	if e != nil {
		t.Fatal(e)
	}
	equal(t, u.User.Username(), "fake%2Fuuid")
}
func TestSourceHTTPSizeAndDeadline(t *testing.T) {
	s := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(403)
		case "/large":
			io.WriteString(w, "123456")
		case "/slow":
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		default:
			io.WriteString(w, "ok")
		}
	}))
	for _, c := range []struct {
		path, why string
		size      int64
	}{{"/status", "source_http_status", 10}, {"/large", "source_too_large", 5}, {"/slow", "source_fetch_failed", 10}} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		_, e := readURL(ctx, s.Client(), s.URL+c.path, c.size, "fixture")
		cancel()
		if e == nil {
			t.Fatal("bad source accepted")
		}
		equal(t, e.Error(), c.why)
	}
}
func TestXrayArchiveChecksumAndSymlinks(t *testing.T) {
	dir := fakeCommands(t)
	_ = dir
	base, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	d := filepath.Join(base, "xray-"+xrayVersion)
	os.Mkdir(d, 0700)
	archive := filepath.Join(d, "Xray-linux-64.zip")
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, _ := z.Create("xray")
	self, _ := os.Executable()
	data, _ := os.ReadFile(self)
	f.Write(data)
	z.Close()
	os.WriteFile(archive, b.Bytes(), 0600)
	_, e = installXray(context.Background(), base, "http://unused.invalid", "bad-checksum")
	if e == nil || e.Error() != "archive_sha256_mismatch" {
		t.Fatal(e)
	}
	h := sha256.Sum256(b.Bytes())
	binary, e := installXray(context.Background(), base, "http://unused.invalid", hex.EncodeToString(h[:]))
	if e != nil {
		t.Fatal(e)
	}
	s, _ := os.Stat(binary)
	equal(t, s.Mode().Perm(), os.FileMode(0700))
	os.Remove(binary)
	os.Symlink(self, binary)
	_, e = installXray(context.Background(), base, "http://unused.invalid", hex.EncodeToString(h[:]))
	if e == nil || e.Error() != "unsafe_binary_path" {
		t.Fatal(e)
	}
	os.Remove(binary)
	os.Remove(archive)
	os.Symlink(self, archive)
	_, e = installXray(context.Background(), base, "http://unused.invalid", hex.EncodeToString(h[:]))
	if e == nil || e.Error() != "unsafe_archive_path" {
		t.Fatal(e)
	}
}

func TestXHTTPMultipleSourceSummariesAndCleanup(t *testing.T) {
	requireSocket(t)
	dir := fakeCommands(t)
	u, q := xNode(t, nil)
	var out bytes.Buffer
	code, e := xhttpSession(context.Background(), &out, xhttpArgs{index: 6, size: 1000, samples: 1, timeout: 2, duration: 10}, filepath.Join(dir, "xray"), u, q, []string{"https://fixture.example/file", "https://failure.example/file?token=fake-secret"})
	if e != nil {
		t.Fatal(e)
	}
	equal(t, code, 1)
	rows := eventRows(t, &out)
	summaries := 0
	for _, r := range rows {
		if r["event"] == "source_summary" {
			summaries++
			if r["source_id"] == float64(1) {
				equal(t, r["successful"], float64(1))
			} else {
				equal(t, r["median_mbps"], nil)
			}
		}
	}
	equal(t, summaries, 2)
	if strings.Contains(out.String(), "fake-secret") || strings.Contains(out.String(), "fake-key") {
		t.Fatal("secret output")
	}
	assertCleanChildren(t)
}
