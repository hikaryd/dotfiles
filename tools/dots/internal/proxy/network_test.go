package proxy

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func fixtureServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	return fixtureHTTP(t, h, false)
}
func fixtureTLS(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	return fixtureHTTP(t, h, true)
}
func fixtureHTTP(t *testing.T, h http.Handler, tls bool) *httptest.Server {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("sandbox blocked real localhost fixture socket: %v", err)
		}
		t.Fatal(err)
	}
	s := &httptest.Server{Listener: l, Config: &http.Server{Handler: h}}
	if tls {
		s.StartTLS()
	} else {
		s.Start()
	}
	t.Cleanup(s.Close)
	return s
}
func TestSubscriptionStreamingAndCache(t *testing.T) {
	c := testController(t)
	link := "vless://fixture-credential@example.test:443?type=xhttp&path=%2Fapi#Fixture"
	encoded := base64.StdEncoding.EncodeToString([]byte(link + "\nnot-vless\n"))
	userAgents := make(chan string, 1)
	s := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgents <- r.Header.Get("User-Agent")
		fmt.Fprint(w, encoded[:20])
		w.(http.Flusher).Flush()
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(w, encoded[20:])
	}))
	if err := c.saveSubscription(s.URL + "/subscription?token=fixture-secret"); err != nil {
		t.Fatal(err)
	}
	servers, err := c.updateServers(false)
	if err != nil {
		t.Fatal(err)
	}
	ua := <-userAgents
	if ua != "V2Box/1.0" || len(servers) != 1 || servers[0]["uuid"] != "fixture-credential" {
		t.Fatalf("ua %s servers %#v", ua, servers)
	}
	cached, err := c.loadServers()
	if err != nil || len(cached) != 1 || cached[0]["raw_url"] != link {
		t.Fatalf("cached %#v %v", cached, err)
	}
	if strings.Contains(c.out.(*bytes.Buffer).String(), "fixture-secret") {
		t.Fatal("secret output")
	}
}
func TestSubscriptionTLSSecureByDefaultAndRedaction(t *testing.T) {
	c := testController(t)
	s := fixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "vless://fixture@example.test:443#TLS") }))
	defer s.Close()
	secretURL := s.URL + "/fixture-secret-token"
	if _, err := c.fetchSubscription(secretURL, false); err == nil || strings.Contains(err.Error(), "fixture-secret-token") {
		t.Fatalf("TLS not verified or leaked error: %v", err)
	}
	links, err := c.fetchSubscription(secretURL, true)
	if err != nil || len(links) != 1 {
		t.Fatalf("override: %v %v", links, err)
	}
	if !strings.Contains(c.errOut.(*bytes.Buffer).String(), "DISABLED") {
		t.Fatal("missing insecure warning")
	}
	s.Close()
	if _, err := c.fetchSubscription(secretURL, false); err == nil || strings.Contains(err.Error(), "fixture-secret-token") {
		t.Fatalf("failed connection leaked: %v", err)
	}
}
func TestSubscriptionHTTPFailurePreservesCache(t *testing.T) {
	c := testController(t)
	s := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); fmt.Fprint(w, "fixture-body-secret") }))
	if err := c.saveSubscription(s.URL + "/?fixture-query-secret"); err != nil {
		t.Fatal(err)
	}
	if err := c.saveServers([]object{{"name": "existing"}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(c.path("servers.json"))
	_, err := c.updateServers(false)
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error %v", err)
	}
	after, _ := os.ReadFile(c.path("servers.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed subscription destroyed cache")
	}
}
func TestStatusDoesNotProbeBrowserChallenges(t *testing.T) {
	c := testController(t)
	var mu sync.Mutex
	var destinations []string
	proxy := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		destinations = append(destinations, r.Host)
		mu.Unlock()
		// A browser challenge must not cause status to try other web endpoints.
		w.WriteHeader(http.StatusForbidden)
	}))
	u, _ := url.Parse(proxy.URL)
	c.port, _ = strconv.Atoi(u.Port())
	if err := c.status(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(destinations) != 1 || destinations[0] != "api.openai.com:443" {
		t.Fatalf("status must check only the API, got %v", destinations)
	}
	if !strings.Contains(c.out.(*bytes.Buffer).String(), "Proxy check FAIL: https://api.openai.com/v1/models") {
		t.Fatal("API failure was not reported")
	}
}
func TestProxyCheckForwardingAndHTTPStatuses(t *testing.T) {
	c := testController(t)
	var mu sync.Mutex
	var gotURI, gotAgent string
	origin := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		w.WriteHeader(code)
	}))
	target, _ := url.Parse(origin.URL)
	forward := httputil.NewSingleHostReverseProxy(target)
	proxy := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotURI = r.RequestURI
		gotAgent = r.Header.Get("User-Agent")
		mu.Unlock()
		if !r.URL.IsAbs() {
			t.Error("request was not sent as a forward proxy request")
		}
		forward.ServeHTTP(w, r)
	}))
	u, _ := url.Parse(proxy.URL)
	c.port, _ = strconv.Atoi(u.Port())
	// Even an all-matching ambient NO_PROXY must not bypass this explicit proxy.
	t.Setenv("NO_PROXY", "*")
	for _, code := range []int{200, 301, 401, 403, 404, 429, 500, 503} {
		targetURL := origin.URL + "/" + strconv.Itoa(code)
		ok, result := c.checkURL(targetURL)
		if ok != (code == 200 || code == 401) || result != fmt.Sprintf("HTTP %d", code) {
			t.Fatalf("%d: %v %s", code, ok, result)
		}
		mu.Lock()
		uri, agent := gotURI, gotAgent
		mu.Unlock()
		if uri != targetURL || agent != "pi-proxy-check/1.0" {
			t.Fatalf("request %s %s", uri, agent)
		}
	}
}
func TestProxyCheckCONNECTFailsClosedAndRedacts(t *testing.T) {
	c := testController(t)
	connected := make(chan string, 1)
	proxy := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("method %s", r.Method)
		}
		connected <- r.Host
		w.Header().Set("Proxy-Authenticate", `Basic realm="fixture-secret"`)
		w.WriteHeader(407)
		fmt.Fprint(w, "fixture-secret")
	}))
	u, _ := url.Parse(proxy.URL)
	c.port, _ = strconv.Atoi(u.Port())
	ok, result := c.checkURL("https://example.invalid/fixture-secret")
	if ok || strings.Contains(result, "fixture-secret") {
		t.Fatalf("not fail closed: %v %s", ok, result)
	}
	if got := <-connected; got != "example.invalid:443" {
		t.Fatal(got)
	}
}
func TestFixtureCONNECTTunnelWithStreaming(t *testing.T) {
	// xray owns the server side in this snapshot. This fixture exercises the
	// native status client's CONNECT request and tunneled TLS protocol, failing
	// closed on an untrusted certificate without contacting any external host.
	c := testController(t)
	origin := fixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "never accepted") }))
	defer origin.Close()
	connected := make(chan struct{}, 1)
	tunnel := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			http.Error(w, "CONNECT required", 405)
			return
		}
		upstream, err := net.DialTimeout("tcp", origin.Listener.Addr().String(), time.Second)
		if err != nil {
			http.Error(w, "fixture failure", 502)
			return
		}
		defer upstream.Close()
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		upstream.SetDeadline(time.Now().Add(3 * time.Second))
		fmt.Fprint(rw, "HTTP/1.1 200 Connection Established\r\n\r\n")
		rw.Flush()
		connected <- struct{}{}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, rw); close(done) }()
		_, _ = io.Copy(conn, bufio.NewReader(upstream))
		conn.Close()
		<-done
	}))
	u, _ := url.Parse(tunnel.URL)
	c.port, _ = strconv.Atoi(u.Port())
	ok, result := c.checkURL("https://example.invalid/fixture-secret")
	if ok || strings.Contains(result, "fixture-secret") {
		t.Fatalf("untrusted tunnel accepted: %v %s", ok, result)
	}
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("no CONNECT")
	}
}
