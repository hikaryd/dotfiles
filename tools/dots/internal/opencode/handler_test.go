package opencode

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func request(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:6446"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestSystemoneTransparentWithoutLocalAuth(t *testing.T) {
	body := `{"model":"jev-1.13-free","state":"sample","questions":{"q":{"type":"noul","instructions":"relevant?"}}}`
	var count atomic.Int32
	var session string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/zen/v1/systemone" || string(b) != body {
			t.Error("changed systemone protocol")
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("wrong upstream key")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("X-Api-Key") != "" {
			t.Error("leaked client credentials")
		}
		if r.Header.Get("X-Opencode-Request") == "" || r.Header.Get("X-Opencode-Session") == "spoofed" {
			t.Error("untrusted metadata")
		}
		if session != "" && session != r.Header.Get("X-Opencode-Session") {
			t.Error("session changed prematurely")
		}
		session = r.Header.Get("X-Opencode-Session")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"answers":{"q":{"type":"noul","noul":0.9}}}`)
	}))
	defer upstream.Close()
	h := newHandler(func() (Config, error) { return testConfig(), nil }, upstream.Client(), upstream.URL+"/zen/v1")
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "http://127.0.0.1:6446/v1/systemone", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if i == 1 {
			r.Header.Set("Authorization", "Bearer attacker")
			r.Header.Set("Cookie", "secret")
			r.Header.Set("X-Api-Key", "attacker")
			r.Header.Set("X-Opencode-Session", "spoofed")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"noul":0.9`) {
			t.Fatalf("response %d: %s", w.Code, w.Body)
		}
	}
	if count.Load() != 2 {
		t.Fatal("wrong request count")
	}
}
func TestHandlerGuards(t *testing.T) {
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer u.Close()
	h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL)
	for _, tc := range []struct {
		method, path, body, host, origin, content string
		code                                      int
	}{
		{"POST", "/v1/systemone", `{"model":"jev-1.13"}`, "", "", "application/json", 400},
		{"POST", "/v1/chat/completions", `{"model":"jev-1.13-free"}`, "", "", "application/json", 400},
		{"POST", "/v1/chat/completions", `{"model":"paid-model"}`, "", "", "application/json", 400},
		{"GET", "/v1/systemone", "", "", "", "application/json", 405},
		{"POST", "/arbitrary", `{}`, "", "", "application/json", 404},
		{"POST", "/v1/systemone", `{}`, "evil.example", "", "application/json", 403},
		{"POST", "/v1/systemone", `{}`, "", "https://evil.example", "application/json", 403},
		{"POST", "/v1/systemone", `{}`, "", "", "text/plain", 415},
		{"POST", "/v1/systemone", `{`, "", "", "application/json", 400},
		{"POST", "/v1/systemone", strings.Repeat(" ", 10*1024*1024+1), "", "", "application/json", 413},
	} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1:6446"+tc.path, strings.NewReader(tc.body))
		if tc.host != "" {
			r.Host = tc.host
		}
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, w.Code, tc.code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached upstream")
	}
	for _, p := range []string{"/health", "/v1/models"} {
		w := request(h, "GET", p, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), "test-secret") {
			t.Fatal("unsafe local endpoint")
		}
	}
}
func TestHandlerAliasErrorsAndKeyReload(t *testing.T) {
	c := testConfig()
	fail := false
	status := 200
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"deepseek-v4-flash-free"`) {
			t.Error("missing alias")
		}
		if r.Header.Get("Authorization") != "Bearer "+c.APIKey {
			t.Error("stale key")
		}
		w.Header().Set("Retry-After", "15")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, `{"message":"test-secret"}`)
	}))
	defer u.Close()
	h := newHandler(func() (Config, error) {
		if fail {
			return Config{}, errors.New("test-secret")
		}
		return c, nil
	}, u.Client(), u.URL)
	for _, s := range []int{200, 401, 403, 429, 500} {
		status = s
		c.APIKey = "changed-key"
		w := request(h, "POST", "/v1/chat/completions", `{"model":"big-pickle","messages":[]}`)
		if w.Code != s {
			t.Fatalf("lost status %d", s)
		}
		if s >= 400 && (strings.Contains(w.Body.String(), "test-secret") || w.Header().Get("Retry-After") != "15") {
			t.Fatal("unsafe upstream error")
		}
	}
	fail = true
	w := request(h, "POST", "/v1/chat/completions", `{"model":"big-pickle"}`)
	if w.Code != 503 || strings.Contains(w.Body.String(), "test-secret") {
		t.Fatal("unsafe reload failure")
	}
}
func TestStreamingFlushAndCancellation(t *testing.T) {
	cancelled := make(chan struct{})
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	defer u.Close()
	h := httptest.NewServer(newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL))
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "POST", h.URL+"/v1/chat/completions", strings.NewReader(`{"model":"big-pickle","stream":true}`))
	r.Header.Set("Content-Type", "application/json")
	res, err := h.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n" {
		t.Fatalf("no early flush: %q %v", line, err)
	}
	cancel()
	res.Body.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream not cancelled")
	}
}
func TestProductionTransportNoRedirectOrDirectFallback(t *testing.T) {
	c, err := upstreamClient("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	if c.CheckRedirect == nil {
		t.Fatal("redirects enabled")
	}
	req, _ := http.NewRequest("GET", "https://opencode.ai/zen/v1/models", nil)
	if c.CheckRedirect(req, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect allowed")
	}
	p, err := c.Transport.(*http.Transport).Proxy(req)
	if err != nil || p.String() != "http://127.0.0.1:1" {
		t.Fatal("proxy bypass")
	}
	c.Timeout = 100 * time.Millisecond
	if res, err := c.Do(req); err == nil {
		res.Body.Close()
		t.Fatal("direct fallback")
	}
}
func TestInterruptedUpstreamAbortsResponse(t *testing.T) {
	for _, content := range []string{"application/json", "text/event-stream"} {
		t.Run(content, func(t *testing.T) {
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				part := `{"partial":true}`
				if content == "text/event-stream" {
					part = "data: {\"partial\":true}\n\n"
				}
				fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Type: %s\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", content, len(part), part)
				rw.Flush() // Missing final chunk is a transport failure, not a complete response.
			}))
			defer u.Close()
			h := httptest.NewServer(newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL))
			defer h.Close()
			res, err := h.Client().Post(h.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"big-pickle","stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if _, err := io.ReadAll(res.Body); err == nil && res.StatusCode < 400 {
				t.Fatal("upstream failure became a successful downstream EOF")
			}
		})
	}
}

func TestResponseHeadersCannotReflectKey(t *testing.T) {
	for _, status := range []int{200, 429} {
		u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; key="+testConfig().APIKey)
			w.Header().Set("Retry-After", testConfig().APIKey)
			w.WriteHeader(status)
			io.WriteString(w, `{}`)
		}))
		h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL)
		w := request(h, "POST", "/v1/chat/completions", `{"model":"big-pickle"}`)
		u.Close()
		if w.Code != status {
			t.Fatalf("wrong status: %d", w.Code)
		}
		if strings.Contains(fmt.Sprint(w.Header()), "test-secret") {
			t.Fatalf("key reflected in headers: %v", w.Header())
		}
	}
}

func TestOpenAIStreamErrorIsSanitized(t *testing.T) {
	for _, event := range []string{"event: error\n", ""} {
		t.Run(event, func(t *testing.T) {
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"safe\"}}]}\n\n")
				w.(http.Flusher).Flush()
				for _, part := range []string{event + "data: {\"er", "ror\":{\"message\":\"test-secret\"}}\n\n"} {
					io.WriteString(w, part)
					w.(http.Flusher).Flush()
				}
			}))
			defer u.Close()
			h := httptest.NewServer(newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL))
			defer h.Close()
			res, err := h.Client().Post(h.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"big-pickle","stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			if strings.Contains(string(b), "test-secret") || !strings.Contains(string(b), "safe") {
				t.Fatalf("unsafe or lost stream: %s", b)
			}
		})
	}
}

func TestSSEPassthroughFramingAndCompletion(t *testing.T) {
	input := ": keepalive\r\nid: 7\r\nevent: message\r\ndata: {\"x\":\r\ndata: 1}\r\n\r\ndata: [DONE]\r\n\r\n"
	want := ": keepalive\nid: 7\nevent: message\ndata: {\"x\":\ndata: 1}\n\ndata: [DONE]\n\n"
	w := httptest.NewRecorder()
	if err := relaySSE(w, strings.NewReader(input)); err != nil || w.Body.String() != want {
		t.Fatalf("successful SSE framing lost: %v %q", err, w.Body.String())
	}
	if err := relaySSE(httptest.NewRecorder(), strings.NewReader("data: {}\n\n")); err == nil {
		t.Fatal("stream without completion accepted")
	}
	if err := relaySSE(httptest.NewRecorder(), strings.NewReader(":"+strings.Repeat("x", 1<<20)+"\n\n")); err == nil {
		t.Fatal("unbounded SSE comment accepted")
	}
}

func TestJSONErrorSanitizedAndSuccessPreserved(t *testing.T) {
	for _, route := range []struct{ path, model string }{{"/v1/chat/completions", "big-pickle"}, {"/v1/systemone", jevModel}} {
		t.Run(route.path, func(t *testing.T) {
			body := `{"error":{"message":"test-secret"}}`
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, body)
			}))
			defer u.Close()
			h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL)
			for _, invalid := range []string{body, `{"broken":"test-secret"`, strings.Repeat("x", maxBody+1)} {
				body = invalid
				w := request(h, "POST", route.path, fmt.Sprintf(`{"model":%q}`, route.model))
				if w.Code != 502 || strings.Contains(w.Body.String(), "test-secret") {
					t.Fatal("unsafe JSON error or invalid response accepted")
				}
			}
			body = " { \"answers\": {\"q\": {\"type\":\"noul\",\"noul\":0.9}} }\n"
			w := request(h, "POST", route.path, fmt.Sprintf(`{"model":%q}`, route.model))
			if w.Code != 200 || w.Body.String() != body {
				t.Fatal("successful JSON changed")
			}
		})
	}
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineRecorder) SetWriteDeadline(d time.Time) error { w.deadline = d; return nil }
func TestAnthropicJSONWriteIsBounded(t *testing.T) {
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`)
	}))
	defer u.Close()
	h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL)
	r := httptest.NewRequest("POST", "http://127.0.0.1:6446/v1/messages", strings.NewReader(`{"model":"big-pickle","messages":[{"role":"user","content":"hi"}]}`))
	r.Header.Set("Content-Type", "application/json")
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, r)
	if w.Code != 200 || !w.deadline.After(time.Now()) {
		t.Fatal("Anthropic response has no downstream write deadline")
	}
}

func TestTransportTimeoutStatus(t *testing.T) {
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body) // Allow net/http to monitor the disconnected socket.
		<-r.Context().Done()
	}))
	defer u.Close()
	c := u.Client()
	c.Timeout = 30 * time.Millisecond
	h := newHandler(func() (Config, error) { return testConfig(), nil }, c, u.URL)
	w := request(h, "POST", "/v1/chat/completions", `{"model":"big-pickle"}`)
	if w.Code != 504 {
		t.Fatal(fmt.Sprint("timeout status ", w.Code))
	}
}
