package beatapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func beatConfig() Config { c := testConfig(); c.APIKey = "beat-secret"; return c }

var beatChatIDs = []string{"glm-5.3-free", "glm-5.3-flash-free", "deepseek-v4.1-flash-free", "deepseek-v4-flash-0731-free", "gemini-3.8-flash-free", "gpt-6.1-sol-free"}

type beatTransport func(*http.Request) (*http.Response, error)

func (f beatTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func beatResponse(status int, content, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {content}}, Body: io.NopCloser(strings.NewReader(body))}
}
func routedHandler(c Config, transport beatTransport) http.Handler {
	client, _ := upstreamClient(c.ProxyURL)
	client.Transport = transport
	return newHandler(func() (Config, error) { return c, nil }, client, upstreamURL)
}

func TestBeatAPISystemoneFixedDestinationAndSeparateKey(t *testing.T) {
	body := `{"model":"jev-1.14-free","state":{"source":"merchant checkout outage"},"questions":{"relevant":{"type":"noul","instructions":"Is the source relevant?"}}}`
	calls := 0
	h := routedHandler(beatConfig(), func(r *http.Request) (*http.Response, error) {
		calls++
		b, _ := io.ReadAll(r.Body)
		if r.URL.String() != "https://api.beatapi.io/v1/systemone" || string(b) != body {
			t.Errorf("wrong fixed destination/protocol: %s %s", r.URL, b)
		}
		if r.Header.Get("Authorization") != "Bearer beat-secret" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("Cookie") != "" {
			t.Error("wrong credential boundary")
		}
		for name := range r.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-opencode-") {
				t.Error("OpenCode metadata leaked to BeatAPI")
			}
		}
		if strings.Contains(r.Header.Get("User-Agent"), "opencode/") {
			t.Error("OpenCode user agent leaked")
		}
		return beatResponse(200, "application/json", `{"answers":{"relevant":{"type":"noul","noul":0.99}}}`), nil
	})
	r := httptest.NewRequest("POST", "http://127.0.0.1:6446/v1/systemone", strings.NewReader(body))
	r.Header = http.Header{"Content-Type": {"application/json"}, "Authorization": {"Bearer attacker"}, "X-Api-Key": {"attacker"}, "Cookie": {"secret"}, "X-Opencode-Session": {"spoofed"}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), `"noul":0.99`) {
		t.Fatalf("typed Jev status=%d calls=%d body=%s", w.Code, calls, w.Body)
	}
}

func TestBeatAPIFreeChatAndAnthropicTools(t *testing.T) {
	for _, id := range beatChatIDs {
		for _, endpoint := range []string{"/v1/chat/completions", "/v1/messages"} {
			t.Run(id+endpoint, func(t *testing.T) {
				calls := 0
				h := routedHandler(beatConfig(), func(r *http.Request) (*http.Response, error) {
					calls++
					b, _ := io.ReadAll(r.Body)
					var got struct {
						Model    string
						Tools    []json.RawMessage
						Messages []json.RawMessage
					}
					if json.Unmarshal(b, &got) != nil || got.Model != id || len(got.Messages) != 1 || len(got.Tools) != 1 || r.URL.String() != "https://api.beatapi.io/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer beat-secret" {
						t.Errorf("incorrect chat route: %s %s", r.URL, b)
					}
					return beatResponse(200, "application/json", `{"id":"chat-1","choices":[{"message":{"content":"hi","tool_calls":[{"id":"c1","function":{"name":"find","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`), nil
				})
				tools := `[{"type":"function","function":{"name":"find","parameters":{"type":"object"}}}]`
				if endpoint == "/v1/messages" {
					tools = `[{"name":"find","input_schema":{"type":"object"}}]`
				}
				w := request(h, "POST", endpoint, fmt.Sprintf(`{"model":%q,"max_tokens":20,"messages":[{"role":"user","content":"hi"}],"tools":%s}`, id, tools))
				if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), "find") {
					t.Fatalf("chat/tools: %d %s", w.Code, w.Body)
				}
				if endpoint == "/v1/messages" && (!strings.Contains(w.Body.String(), `"tool_use"`) || !strings.Contains(w.Body.String(), `"model":"`+id+`"`)) {
					t.Fatal("lost Anthropic tools/model")
				}
			})
		}
	}
}

func TestBeatAPIRejectsPaidAliasesAndCrossEndpointWithoutRequests(t *testing.T) {
	calls := 0
	h := routedHandler(beatConfig(), func(*http.Request) (*http.Response, error) {
		calls++
		return beatResponse(200, "application/json", `{}`), nil
	})
	for _, id := range []string{"glm-5.3", "deepseek-v4.1-flash", "gpt-6.1-sol", "typesafe/jev-1.14", "typesafe/jev-1.14:free", "jev-1.14", "latest", "auto", "made-up-free", "beatapi/big-pickle", "beatapi/opencode/glm-5.3-free", "opencode/beatapi/glm-5.3-free", "beatapi/../glm-5.3-free", "jev-1.14-free", "beatapi/jev-1.14-free"} {
		for _, path := range []string{"/v1/chat/completions", "/v1/messages"} {
			if w := request(h, "POST", path, fmt.Sprintf(`{"model":%q}`, id)); w.Code != 400 {
				t.Errorf("accepted %s %s: %d", path, id, w.Code)
			}
		}
	}
	for _, id := range []string{"glm-5.3-free", "beatapi/glm-5.3-free", "opencode/big-pickle", "jev-1.14", "beatapi/jev-1.13-free", "opencode/jev-1.14-free"} {
		if w := request(h, "POST", "/v1/systemone", fmt.Sprintf(`{"model":%q}`, id)); w.Code != 400 {
			t.Errorf("accepted cross endpoint %s: %d", id, w.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid request reached network %d times", calls)
	}
}

func TestBeatAPIModelCatalogMatchesActualProvider(t *testing.T) {
	h := routedHandler(beatConfig(), func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.beatapi.io" {
			t.Error("catalog route escaped BeatAPI")
		}
		return beatResponse(200, "application/json", `{}`), nil
	})
	w := request(h, "GET", "/v1/models", "")
	var catalog struct {
		Data []struct {
			ID    string
			Owned string `json:"owned_by"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &catalog) != nil || len(catalog.Data) != 14 {
		t.Fatalf("wrong catalog: %s", w.Body)
	}
	seen := map[string]bool{}
	for _, item := range catalog.Data {
		if seen[item.ID] || item.Owned != "beatapi" || strings.HasPrefix(item.ID, "opencode/") {
			t.Fatal("catalog not BeatAPI-only")
		}
		seen[item.ID] = true
		path := "/v1/chat/completions"
		if strings.Contains(item.ID, "jev-") {
			path = "/v1/systemone"
		}
		if response := request(h, "POST", path, fmt.Sprintf(`{"model":%q}`, item.ID)); response.Code != 200 {
			t.Fatalf("unusable catalog model: %s", item.ID)
		}
	}
}

func TestBeatAPIErrorsAreSanitizedAndNeverRetried(t *testing.T) {
	for _, status := range []int{302, 401, 403, 429, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			calls := 0
			h := routedHandler(beatConfig(), func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "api.beatapi.io" || r.Header.Get("Authorization") != "Bearer beat-secret" {
					t.Error("retried/cross-routed")
				}
				res := beatResponse(status, "application/json", `{"error":{"message":"beat-secret test-secret\u001b[31m"}}`)
				res.Header.Set("Retry-After", "00015")
				res.Header.Set("Location", "https://attacker.example/")
				return res, nil
			})
			w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
			if w.Code != status || calls != 1 || !strings.Contains(w.Body.String(), "BeatAPI") || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Retry-After") != "15" || w.Header().Get("Location") != "" {
				t.Fatalf("unsafe failure %d calls=%d %s", w.Code, calls, w.Body)
			}
		})
	}
	for _, mode := range []string{"transport", "header", "json-error", "bad-json"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			h := routedHandler(beatConfig(), func(*http.Request) (*http.Response, error) {
				calls++
				switch mode {
				case "transport":
					return nil, errors.New("beat-secret test-secret")
				case "header":
					return beatResponse(200, "beat-secret", `{}`), nil
				case "json-error":
					return beatResponse(200, "application/json", `{"error":{"message":"beat-secret"}}`), nil
				default:
					return beatResponse(200, "application/json", `{"unfinished":`), nil
				}
			})
			w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
			if w.Code != 502 || calls != 1 || strings.Contains(w.Body.String()+w.Header().Get("Content-Type"), "secret") || !strings.Contains(w.Body.String(), "BeatAPI") {
				t.Fatalf("unsafe %s: %d %s", mode, w.Code, w.Body)
			}
		})
	}
}

func TestBeatAPIStreamsUseExistingOpenAIAndAnthropicRelay(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/messages"} {
		calls := 0
		h := routedHandler(beatConfig(), func(r *http.Request) (*http.Response, error) {
			calls++
			return beatResponse(200, "text/event-stream", "data: {\"id\":\"chat-1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"), nil
		})
		w := request(h, "POST", path, `{"model":"beatapi/glm-5.3-free","stream":true,"max_tokens":20,"messages":[{"role":"user","content":"hi"}]}`)
		if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), "hello") {
			t.Fatalf("SSE relay %s: %d %s", path, w.Code, w.Body)
		}
		if path == "/v1/messages" && !strings.Contains(w.Body.String(), "message_stop") {
			t.Fatal("Anthropic stream incomplete")
		}
	}
}

func TestBeatAPIHealthRequiresOwnServiceAndCapability(t *testing.T) {
	for _, tc := range []struct {
		service       string
		capable, want bool
	}{{"local.dots.opencode-proxy", true, false}, {label, false, false}, {label, true, true}} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capabilities := `[]`
			if tc.capable {
				capabilities = `["beatapi"]`
			}
			fmt.Fprintf(w, `{"service":%q,"configured":true,"capabilities":%s}`, tc.service, capabilities)
		}))
		cfg := testConfig()
		cfg.Port, _ = strconv.Atoi(strings.TrimPrefix(s.URL, "http://127.0.0.1:"))
		got := localHealth(cfg)
		s.Close()
		if got != tc.want {
			t.Errorf("foreign/unsupported API accepted: %s capability=%t", tc.service, tc.capable)
		}
	}
}
