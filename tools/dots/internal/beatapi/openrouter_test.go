package beatapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const decisionRequest = `{"model":"jev-1.14","state":{"id":9007199254740993,"source":"jev-1.14 must stay unchanged"},"questions":{"relevant":{"type":"noul","instructions":"Relevant?"}},"extra":[null,9007199254740993]}`
const decisionAnswer = `{"model":"liquid/d1-ultra","answers":{"relevant":{"type":"noul","noul":0.99}},"usage":{"input_tokens":10,"output_tokens":0}}`

func TestOpenRouterPrimaryPreservesTypedPayloadAndSeparatesKeys(t *testing.T) {
	for _, model := range []string{"jev-1.14", "beatapi/jev-1.14", "jev-1.14-free", "beatapi/jev-1.14-free"} {
		t.Run(model, func(t *testing.T) {
			calls := 0
			h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://openrouter.ai/api/alpha/decisions" || r.Method != "POST" {
					t.Errorf("wrong primary endpoint: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer openrouter-secret" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("Cookie") != "" {
					t.Error("primary received the wrong credentials")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 10*time.Second {
					t.Error("primary is not bounded to 10 seconds")
				}
				b, _ := io.ReadAll(r.Body)
				var got, want map[string]json.RawMessage
				json.Unmarshal(b, &got)
				json.Unmarshal([]byte(decisionRequest), &want)
				want["model"] = json.RawMessage(`"liquid/d1-ultra"`)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("changed decision fields or integer precision: %s", b)
				}
				return beatResponse(200, "application/json", decisionAnswer), nil
			})
			body := strings.Replace(decisionRequest, `"model":"jev-1.14"`, fmt.Sprintf(`"model":%q`, model), 1)
			r := httptest.NewRequest("POST", "http://127.0.0.1:6446/v1/systemone", strings.NewReader(body))
			r.Header = http.Header{"Content-Type": {"application/json"}, "Authorization": {"Bearer attacker"}, "Cookie": {"attacker-cookie"}, "X-Api-Key": {"attacker-key"}}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 || calls != 1 || w.Body.String() != decisionAnswer || w.Header().Get("X-Dots-Jev-Provider") != "OpenRouter" || w.Header().Get("X-Dots-Jev-Fallback") != "" {
				t.Fatalf("primary response: status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body)
			}
		})
	}
}

func TestOpenRouterHTTPFailuresFallBackAtMostOnce(t *testing.T) {
	for _, status := range []int{302, 400, 401, 402, 403, 404, 408, 413, 422, 429, 500, 503, 599} {
		for _, fallbackStatus := range []int{200, 503} {
			t.Run(fmt.Sprintf("%d/%d", status, fallbackStatus), func(t *testing.T) {
				calls := 0
				h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(r *http.Request) (*http.Response, error) {
					calls++
					b, _ := io.ReadAll(r.Body)
					if calls == 1 {
						if r.URL.Host != "openrouter.ai" || r.Header.Get("Authorization") != "Bearer openrouter-secret" {
							t.Error("first attempt was not OpenRouter")
						}
						res := beatResponse(status, "application/json", `{"error":{"message":"openrouter-secret test-secret"}}`)
						res.Header.Set("Location", "https://attacker.example/")
						return res, nil
					}
					if r.URL.String() != "https://api.beatapi.io/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-secret" || string(b) != decisionRequest {
						t.Errorf("fallback changed credentials or request: %s %s", r.URL, b)
					}
					return beatResponse(fallbackStatus, "application/json", decisionAnswer), nil
				})
				w := request(h, "POST", "/v1/systemone", decisionRequest)
				wantCalls, wantStatus := 1, status
				if status == 408 || status == 429 || status >= 500 {
					wantCalls, wantStatus = 2, fallbackStatus
					if w.Header().Get("X-Dots-Jev-Fallback") != "true" {
						t.Error("fallback was not reported")
					}
				}
				if calls != wantCalls || w.Code != wantStatus || w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), "secret") {
					t.Fatalf("unsafe retry/rejection: calls=%d status=%d body=%s", calls, w.Code, w.Body)
				}
				if wantCalls == 1 && !strings.Contains(w.Body.String(), "OpenRouter") {
					t.Error("primary rejection was attributed to BeatAPI")
				}
			})
		}
	}
}

func TestOpenRouterTransportErrorsFallBackOnce(t *testing.T) {
	for _, transportErr := range []error{os.ErrDeadlineExceeded, errors.New("openrouter-secret network failure")} {
		calls := 0
		h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return nil, transportErr
			}
			if r.URL.Host != "api.beatapi.io" || r.Context().Err() != nil {
				t.Error("fallback inherited the failed attempt's context or destination")
			}
			return beatResponse(200, "application/json", decisionAnswer), nil
		})
		w := request(h, "POST", "/v1/systemone", decisionRequest)
		if calls != 2 || w.Code != 200 || w.Header().Get("X-Dots-Jev-Provider") != "BeatAPI" {
			t.Fatalf("transport fallback: calls=%d status=%d body=%s", calls, w.Code, w.Body)
		}
	}
}

func TestOpenRouterInvalidResponsesFallBackBeforeSendingClientBytes(t *testing.T) {
	for _, tc := range []struct{ name, content, body string }{
		{"empty", "application/json", ""},
		{"invalid", "application/json", "{"},
		{"null", "application/json", "null"},
		{"no-answers", "application/json", `{}`},
		{"partial", "application/json", `{"answers":{"other":{"type":"noul","noul":0.9}}}`},
		{"null-answer", "application/json", `{"answers":{"relevant":null}}`},
		{"missing-noul", "application/json", `{"answers":{"relevant":{"type":"noul"}}}`},
		{"null-noul", "application/json", `{"answers":{"relevant":{"type":"noul","noul":null}}}`},
		{"wrong-type", "application/json", `{"answers":{"relevant":{"type":"choice","noul":0.9}}}`},
		{"invalid-probability", "application/json", `{"answers":{"relevant":{"type":"noul","noul":2}}}`},
		{"error", "application/json", `{"error":{"message":"openrouter-secret"}}`},
		{"html", "text/html", decisionAnswer},
		{"too-large", "application/json", strings.Repeat(" ", maxBody+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return beatResponse(200, tc.content, tc.body), nil
				}
				return beatResponse(200, "application/json", decisionAnswer), nil
			})
			w := request(h, "POST", "/v1/systemone", decisionRequest)
			if calls != 2 || w.Code != 200 || w.Body.String() != decisionAnswer {
				t.Fatalf("bad response escaped fallback: calls=%d status=%d body=%s", calls, w.Code, w.Body)
			}
		})
	}
}

func TestOpenRouterBodyFailureClosesResponseBeforeFallback(t *testing.T) {
	broken := &paidTimedOutBody{}
	calls := 0
	h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: broken}, nil
		}
		if !broken.closed {
			t.Error("primary response was not closed before fallback")
		}
		return beatResponse(200, "application/json", decisionAnswer), nil
	})
	w := request(h, "POST", "/v1/systemone", decisionRequest)
	if w.Code != 200 || calls != 2 || !broken.closed {
		t.Fatalf("body timeout did not fall back safely: status=%d calls=%d closed=%t", w.Code, calls, broken.closed)
	}
}

func TestOpenRouterCallerCancellationNeverStartsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "openrouter.ai" {
			t.Error("cancelled request reached fallback")
		}
		cancel()
		return nil, r.Context().Err()
	})
	r := httptest.NewRequest("POST", "http://127.0.0.1:6446/v1/systemone", strings.NewReader(decisionRequest)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if calls != 1 {
		t.Fatalf("cancellation caused %d attempts", calls)
	}
}

func TestOpenRouterDoesNotRouteChatOrUnlistedModels(t *testing.T) {
	calls := 0
	h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.beatapi.io" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("chat was sent to OpenRouter or given the wrong key")
		}
		return beatResponse(200, "application/json", `{}`), nil
	})
	if w := request(h, "POST", "/v1/chat/completions", `{"model":"glm-5.3-free","messages":[]}`); w.Code != 200 {
		t.Fatal("chat behavior changed")
	}
	for _, model := range []string{"liquid/d1-ultra", "openai/gpt-6-luna-decisions", "https://attacker.example/"} {
		if w := request(h, "POST", "/v1/systemone", fmt.Sprintf(`{"model":%q}`, model)); w.Code != 400 {
			t.Fatal("OpenRouter integration bypassed the local allowlist")
		}
	}
	if calls != 1 {
		t.Fatal("rejected model reached the network")
	}
}

func TestOpenRouterHealthIdentifiesConfiguredRoute(t *testing.T) {
	h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(*http.Request) (*http.Response, error) {
		t.Fatal("health made an upstream request")
		return nil, nil
	})
	w := request(h, "GET", "/health", "")
	var health struct {
		Capabilities []string
		Primary      string `json:"jev_primary"`
		Fallback     string `json:"jev_fallback"`
	}
	if json.Unmarshal(w.Body.Bytes(), &health) != nil || health.Primary != "liquid/d1-ultra" || health.Fallback != "jev-1.14" || !strings.Contains(w.Body.String(), `"openrouter"`) || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("health does not identify the safe route: %s", w.Body)
	}
}

func TestOpenRouterHealthRejectsOldBeatAPIOnlyService(t *testing.T) {
	c := testCLI(t)
	configureDoctor(t, c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"service": label, "configured": true, "capabilities": []string{"beatapi"}})
	}))
	cfg, err := loadConfig(c.config)
	if err != nil {
		t.Fatal(err)
	}
	cfg.OpenRouterAPIKey = "openrouter-secret"
	if localHealth(cfg) {
		t.Fatal("old service was reported ready to use OpenRouter")
	}
}

func TestOpenRouterDoctorReportsActualFallback(t *testing.T) {
	c := testCLI(t)
	configureDoctor(t, c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Dots-Jev-Provider", "BeatAPI")
		w.Header().Set("X-Dots-Jev-Fallback", "true")
		io.WriteString(w, decisionAnswer)
	}))
	cfg, err := loadConfig(c.config)
	if err != nil {
		t.Fatal(err)
	}
	cfg.OpenRouterAPIKey = "openrouter-secret"
	if err := c.doctor(cfg); err != nil {
		t.Fatal(err)
	}
	out := fmt.Sprint(c.out)
	if !strings.Contains(out, "OpenRouter") || !strings.Contains(out, "BeatAPI (fallback)") || strings.Contains(out, "secret") {
		t.Fatalf("doctor hid fallback or leaked a key: %s", out)
	}
}

func TestOpenRouterRealHTTPFallback(t *testing.T) {
	var primaryCalls, fallbackCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls.Add(1)
		io.Copy(io.Discard, r.Body)
		if r.URL.Path != "/api/alpha/decisions" || r.Header.Get("Authorization") != "Bearer openrouter-secret" {
			t.Error("real HTTP primary received the wrong path or key")
		}
		w.WriteHeader(503)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-secret" || string(b) != decisionRequest {
			t.Error("real HTTP fallback changed the request or credential")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"model":"jev-1.14","answers":{"relevant":{"type":"noul","noul":0.98}}}`)
	}))
	defer fallback.Close()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	h := routedHandler(openRouterConfig(t, "openrouter-secret"), func(r *http.Request) (*http.Response, error) {
		target := fallback.URL
		if r.URL.Host == "openrouter.ai" {
			target = primary.URL
		}
		u, _ := url.Parse(target)
		forwarded := r.Clone(r.Context())
		forwarded.URL.Scheme, forwarded.URL.Host = u.Scheme, u.Host
		return transport.RoundTrip(forwarded)
	})
	w := request(h, "POST", "/v1/systemone", decisionRequest)
	if w.Code != 200 || primaryCalls.Load() != 1 || fallbackCalls.Load() != 1 || !strings.Contains(w.Body.String(), `"noul":0.98`) || w.Header().Get("X-Dots-Jev-Fallback") != "true" {
		t.Fatalf("real HTTP fallback failed: primary=%d fallback=%d status=%d body=%s", primaryCalls.Load(), fallbackCalls.Load(), w.Code, w.Body)
	}
}

func TestOpenRouterDoctorRejectsServiceWithoutProviderReporting(t *testing.T) {
	c := testCLI(t)
	configureDoctor(t, c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, decisionAnswer)
	}))
	cfg, err := loadConfig(c.config)
	if err != nil {
		t.Fatal(err)
	}
	cfg.OpenRouterAPIKey = "openrouter-secret"
	if err := c.doctor(cfg); err == nil || !strings.Contains(err.Error(), "restart") {
		t.Fatal("doctor reported configured route verified without knowing the actual provider")
	}
	if strings.Contains(fmt.Sprint(c.out), "access verified") {
		t.Fatal("doctor falsely reported OpenRouter access verified")
	}
}
