package beatapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpstreamRejectionExplainsQuotaAndAccessWithoutReflectingSecrets(t *testing.T) {
	for _, tc := range []struct {
		status    int
		typ, want string
	}{
		{429, "FreeUsageLimitError", "usage limit"},
		{403, "PermissionError", "rejected the request"},
		{401, "AuthenticationError", "API key"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "59941")
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"type":"error","error":{"type":%q,"message":"test-secret\u001b[31m"}}`, tc.typ)
			}))
			defer u.Close()
			h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL)
			w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "test-secret") || strings.Contains(w.Body.String(), "31m") {
				t.Fatalf("unsafe or unhelpful rejection: %d %s", w.Code, w.Body)
			}
			if w.Header().Get("Retry-After") != "59941" {
				t.Fatal("upstream cooldown changed")
			}
		})
	}
}

func configureDoctor(t *testing.T, c *cli, h http.Handler) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	_, p, _ := net.SplitHostPort(s.Listener.Addr().String())
	cfg := testConfig()
	cfg.Port, _ = strconv.Atoi(p)
	if err := saveConfig(c.config, cfg); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDoctorChecksRealJevAnswerOnceAndDoesNotWriteConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, body, retry, want string
		status, exit            int
	}{
		{"success", `{"answers":{"relevant":{"type":"noul","noul":0.99}}}`, "", "Jev access verified", 200, 0},
		{"quota", `{"error":{"message":"BeatAPI usage limit reached; wait for Retry-After or check account limits"}}`, "59941", "59941", 429, 1},
		{"not-an-answer", `{"configured":true}`, "", "invalid Jev response", 200, 1},
		{"low-score", `{"answers":{"relevant":{"type":"noul","noul":0.1}}}`, "", "invalid Jev response", 200, 1},
		{"infinity-score", `{"answers":{"relevant":{"type":"noul","noul":2}}}`, "", "invalid Jev response", 200, 1},
		{"secret-error", `{"error":{"message":"test-secret"}}`, "", "HTTP 403", 403, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testCLI(t)
			var calls atomic.Int32
			configureDoctor(t, c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				b, _ := io.ReadAll(r.Body)
				if r.Method != "POST" || r.URL.Path != "/v1/systemone" || !strings.Contains(string(b), `"type":"noul"`) || !strings.Contains(string(b), `"model":"jev-1.14"`) || r.Header.Get("Authorization") != "" {
					t.Error("doctor did not exercise the actual local Jev protocol")
				}
				w.Header().Set("Retry-After", tc.retry)
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			before, _ := os.ReadFile(c.config)
			var out, errOut bytes.Buffer
			code := Run([]string{"doctor"}, strings.NewReader(""), &out, &errOut)
			after, _ := os.ReadFile(c.config)
			text := out.String() + errOut.String()
			if code != tc.exit || calls.Load() != 1 || !strings.Contains(text, tc.want) || strings.Contains(text, "test-secret") || !bytes.Equal(before, after) {
				t.Fatalf("doctor exit=%d calls=%d output=%q", code, calls.Load(), text)
			}
		})
	}
}

func TestDoctorCancellationAbortsLocalRequest(t *testing.T) {
	c := testCLI(t)
	started := make(chan struct{})
	configureDoctor(t, c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.ctx = ctx
	ended := make(chan error, 1)
	go func() { ended <- c.execute("doctor", false) }()
	select {
	case <-started:
	case err := <-ended:
		t.Fatalf("doctor never made a request: %v", err)
	case <-time.After(time.Second):
		t.Fatal("doctor never started")
	}
	cancel()
	select {
	case err := <-ended:
		if err == nil {
			t.Fatal("cancelled doctor succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("doctor ignored cancellation")
	}
}

func TestDoctorIfConnectedReportsBrokenLocalConfig(t *testing.T) {
	for _, port := range []int{defaultPort, 16446} {
		for _, damage := range []string{"unsafe-mode", "malformed", "missing"} {
			t.Run(fmt.Sprintf("%d/%s", port, damage), func(t *testing.T) {
				c := testCLI(t)
				cfg := testConfig()
				cfg.Port = port
				if err := saveConfig(c.config, cfg); err != nil {
					t.Fatal(err)
				}
				credentials := filepath.Join(c.base, "jevgrep/credentials.json")
				if err := privateDir(filepath.Dir(credentials)); err != nil {
					t.Fatal(err)
				}
				b := fmt.Appendf(nil, `{"provider":"custom","baseURL":"http://%s/v1","model":"jev-1.14-free","apiKey":"local"}`, cfg.address())
				if err := atomicPrivate(credentials, b); err != nil {
					t.Fatal(err)
				}
				switch damage {
				case "unsafe-mode":
					os.Chmod(c.config, 0644)
				case "malformed":
					os.WriteFile(c.config, []byte("not-json"), 0600)
				case "missing":
					os.Remove(c.config)
				}
				var out, errOut bytes.Buffer
				code := Run([]string{"doctor", "--if-connected"}, strings.NewReader(""), &out, &errOut)
				if code != 1 || !strings.Contains(errOut.String(), "secure configuration") {
					t.Fatalf("broken local config fell through to official doctor: %d %s %s", code, &out, &errOut)
				}
			})
		}
	}
}

func TestDoctorIfConnectedSkipsWithoutConfigOrSideEffects(t *testing.T) {
	c := testCLI(t)
	var out, errOut bytes.Buffer
	if code := Run([]string{"doctor", "--if-connected"}, strings.NewReader(""), &out, &errOut); code != 3 || out.Len()+errOut.Len() != 0 {
		t.Fatalf("non-local jg was intercepted: %d %s %s", code, &out, &errOut)
	}
	if _, err := os.Stat(c.config); !os.IsNotExist(err) {
		t.Fatal("doctor wrote configuration")
	}
}
