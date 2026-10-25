package beatapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJevFallbackFreeFirstStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		paid   bool
	}{
		{200, false}, {302, false}, {400, false}, {401, false}, {402, false},
		{403, false}, {404, false}, {408, false}, {422, false}, {429, true},
		{500, true}, {503, true}, {599, true}, {600, false},
	} {
		for _, model := range []string{"jev-1.14-free", "beatapi/jev-1.14-free"} {
			t.Run(strconv.Itoa(tc.status)+"/"+model, func(t *testing.T) {
				var calls atomic.Int32
				u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					call := calls.Add(1)
					b, _ := io.ReadAll(r.Body)
					var got map[string]json.RawMessage
					if json.Unmarshal(b, &got) != nil {
						t.Errorf("invalid upstream body: %s", b)
					}
					wantModel := `"jev-1.14-free"`
					if call == 2 {
						wantModel = `"jev-1.14"`
					}
					if string(got["model"]) != wantModel {
						t.Errorf("attempt %d model=%s want %s", call, got["model"], wantModel)
					}
					delete(got, "model")
					wantFields := map[string]json.RawMessage{
						"state":     json.RawMessage(`{"source":"a jev-1.14-free reference","id":9007199254740993}`),
						"questions": json.RawMessage(`{"relevant":{"type":"noul","instructions":"Matches?"}}`),
						"extra":     json.RawMessage(`["keep",null,true,9007199254740993]`),
					}
					if !reflect.DeepEqual(got, wantFields) {
						t.Errorf("attempt %d changed typed payload: %s", call, b)
					}
					if r.Method != "POST" || r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-secret" {
						t.Error("fallback changed destination, method or key")
					}
					w.Header().Set("Content-Type", "application/json")
					if call == 1 {
						w.Header().Set("Retry-After", "3600")
						w.WriteHeader(tc.status)
						io.WriteString(w, `{"model":"jev-1.14-free","answers":{"relevant":{"type":"noul","noul":0.8}}}`)
						return
					}
					io.WriteString(w, `{"model":"jev-1.14","answers":{"relevant":{"type":"noul","noul":0.99}}}`)
				}))
				defer u.Close()
				h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL+"/v1")
				body := fmt.Sprintf(`{"model":%q,"state":{"source":"a jev-1.14-free reference","id":9007199254740993},"questions":{"relevant":{"type":"noul","instructions":"Matches?"}},"extra":["keep",null,true,9007199254740993]}`, model)
				w := request(h, "POST", "/v1/systemone", body)
				wantCalls, wantStatus := int32(1), tc.status
				if tc.paid {
					wantCalls, wantStatus = 2, 200
					if !strings.Contains(w.Body.String(), `"model":"jev-1.14"`) || !strings.Contains(w.Body.String(), `"noul":0.99`) || w.Header().Get("Retry-After") != "" {
						t.Errorf("lost paid response or retained free cooldown: %s %v", w.Body, w.Header())
					}
				}
				if calls.Load() != wantCalls || w.Code != wantStatus {
					t.Fatalf("status=%d calls=%d, want status=%d calls=%d", w.Code, calls.Load(), wantStatus, wantCalls)
				}
			})
		}
	}
}

func TestJevFallbackPaidFailureIsTerminalAndSanitized(t *testing.T) {
	for _, status := range []int{401, 402, 429, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			calls := 0
			h := routedHandler(testConfig(), func(r *http.Request) (*http.Response, error) {
				calls++
				b, _ := io.ReadAll(r.Body)
				if calls == 2 && !strings.Contains(string(b), `"model":"jev-1.14"`) {
					t.Errorf("terminal attempt is not paid Jev: %s", b)
				}
				code, retry := 429, "3600"
				if calls > 1 {
					code, retry = status, "00015"
				}
				res := beatResponse(code, "application/json", `{"error":{"message":"test-secret\u001b[31m"}}`)
				res.Header.Set("Retry-After", retry)
				res.Header.Set("Location", "https://attacker.example/")
				return res, nil
			})
			w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
			if w.Code != status || calls != 2 || w.Header().Get("Retry-After") != "15" || w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), "test-secret") {
				t.Fatalf("unsafe/unbounded terminal fallback: status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body)
			}
		})
	}
}

func TestJevFallbackUpstreamHeaderTimeout(t *testing.T) {
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"model":"jev-1.14-free"`) {
			<-r.Context().Done()
			return
		}
		if !strings.Contains(string(b), `"model":"jev-1.14"`) {
			t.Errorf("timeout fallback sent wrong model: %s", b)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"answers":{"relevant":{"type":"noul","noul":0.99}}}`)
	}))
	defer u.Close()
	tr := &http.Transport{ResponseHeaderTimeout: 50 * time.Millisecond}
	defer tr.CloseIdleConnections()
	h := newHandler(func() (Config, error) { return testConfig(), nil }, &http.Client{Transport: tr}, u.URL)
	w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
	if w.Code != 200 || calls.Load() != 2 || !strings.Contains(w.Body.String(), `"noul":0.99`) {
		t.Fatalf("header timeout did not recover: status=%d calls=%d body=%s", w.Code, calls.Load(), w.Body)
	}
}

func TestJevFallbackCallerCancellationNeverSpends(t *testing.T) {
	for _, mode := range []string{"cancelled-timeout", "expired-timeout", "cancelled-429", "cancelled-503"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if mode == "expired-timeout" {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			}
			defer cancel()
			calls := 0
			h := routedHandler(testConfig(), func(*http.Request) (*http.Response, error) {
				calls++
				cancel()
				switch mode {
				case "cancelled-429":
					return beatResponse(429, "application/json", `{}`), nil
				case "cancelled-503":
					return beatResponse(503, "application/json", `{}`), nil
				default:
					return nil, os.ErrDeadlineExceeded
				}
			})
			r := httptest.NewRequest("POST", "http://127.0.0.1:6446/v1/systemone", strings.NewReader(`{"model":"jev-1.14-free"}`)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(httptest.NewRecorder(), r)
			if calls != 1 {
				t.Fatalf("caller cancellation caused %d upstream attempts", calls)
			}
		})
	}
}

func TestJevFallbackNeverSpendsForChatOrGenericNetworkFailure(t *testing.T) {
	for _, tc := range []struct{ path, model string }{
		{"/v1/chat/completions", "glm-5.3-free"},
		{"/v1/messages", "glm-5.3-free"},
		{"/v1/systemone", "jev-1.14-free"},
	} {
		for _, mode := range []string{"429", "503", "timeout", "network"} {
			if tc.path == "/v1/systemone" && mode != "network" {
				continue
			}
			t.Run(tc.path+"/"+mode, func(t *testing.T) {
				calls := 0
				h := routedHandler(testConfig(), func(*http.Request) (*http.Response, error) {
					calls++
					switch mode {
					case "timeout":
						return nil, os.ErrDeadlineExceeded
					case "network":
						return nil, errors.New("test-secret connection refused")
					default:
						status, _ := strconv.Atoi(mode)
						return beatResponse(status, "application/json", `{}`), nil
					}
				})
				w := request(h, "POST", tc.path, fmt.Sprintf(`{"model":%q,"max_tokens":20,"messages":[{"role":"user","content":"hi"}]}`, tc.model))
				if calls != 1 || w.Code < 400 || strings.Contains(w.Body.String(), "test-secret") {
					t.Fatalf("unexpected paid/network retry: calls=%d status=%d body=%s", calls, w.Code, w.Body)
				}
			})
		}
	}
}

type fallbackTrackedBody struct {
	io.Reader
	closed bool
}

func (b *fallbackTrackedBody) Close() error { b.closed = true; return nil }

func TestJevFallbackClosesFreeBodyBeforePaidAttempt(t *testing.T) {
	freeBody := &fallbackTrackedBody{Reader: strings.NewReader("discarded free error")}
	paidBody := &fallbackTrackedBody{Reader: strings.NewReader(`{"answers":{}}`)}
	calls := 0
	h := routedHandler(testConfig(), func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 429, Header: make(http.Header), Body: freeBody}, nil
		}
		if !freeBody.closed {
			t.Error("free response still open when paid inference started")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: paidBody}, nil
	})
	w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
	if w.Code != 200 || calls != 2 || !freeBody.closed || !paidBody.closed {
		t.Fatalf("unclosed or missing attempts: status=%d calls=%d freeClosed=%t paidClosed=%t", w.Code, calls, freeBody.closed, paidBody.closed)
	}
}
