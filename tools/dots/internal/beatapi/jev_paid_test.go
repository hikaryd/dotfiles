package beatapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJevPaidFirstAndLegacyAliases(t *testing.T) {
	for _, model := range []string{"jev-1.14", "beatapi/jev-1.14", "jev-1.14-free", "beatapi/jev-1.14-free"} {
		t.Run(model, func(t *testing.T) {
			var calls atomic.Int32
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				b, _ := io.ReadAll(r.Body)
				var got map[string]json.RawMessage
				if json.Unmarshal(b, &got) != nil || string(got["model"]) != `"jev-1.14"` {
					t.Errorf("first attempt did not use paid Jev: %s", b)
				}
				delete(got, "model")
				want := map[string]json.RawMessage{
					"state":     json.RawMessage(`{"source":"a jev-1.14-free reference","id":9007199254740993}`),
					"questions": json.RawMessage(`{"relevant":{"type":"noul","instructions":"Matches?"}}`),
					"extra":     json.RawMessage(`["keep",null,true,9007199254740993]`),
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("changed typed payload: %s", b)
				}
				if r.Method != "POST" || r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("paid request changed endpoint, method or key")
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"model":"jev-1.14","answers":{"relevant":{"type":"noul","noul":0.99}}}`)
			}))
			defer u.Close()
			h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL+"/v1")
			body := fmt.Sprintf(`{"model":%q,"state":{"source":"a jev-1.14-free reference","id":9007199254740993},"questions":{"relevant":{"type":"noul","instructions":"Matches?"}},"extra":["keep",null,true,9007199254740993]}`, model)
			w := request(h, "POST", "/v1/systemone", body)
			if w.Code != 200 || calls.Load() != 1 || !strings.Contains(w.Body.String(), `"model":"jev-1.14"`) {
				t.Fatalf("paid route: status=%d calls=%d body=%s", w.Code, calls.Load(), w.Body)
			}
		})
	}
}

func TestJevPaidErrorsNeverRetryAndAreSanitized(t *testing.T) {
	for _, model := range []string{"jev-1.14", "jev-1.14-free"} {
		for _, status := range []int{302, 400, 401, 402, 403, 404, 408, 422, 429, 500, 503, 599} {
			t.Run(model+"/"+strconv.Itoa(status), func(t *testing.T) {
				calls := 0
				h := routedHandler(testConfig(), func(r *http.Request) (*http.Response, error) {
					calls++
					b, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(b), `"model":"jev-1.14"`) || r.URL.String() != "https://api.beatapi.io/v1/systemone" {
						t.Errorf("wrong paid route: %s %s", r.URL, b)
					}
					res := beatResponse(status, "application/json", `{"error":{"message":"test-secret\u001b[31m"}}`)
					res.Header.Set("Retry-After", "00015")
					res.Header.Set("Location", "https://attacker.example/")
					return res, nil
				})
				w := request(h, "POST", "/v1/systemone", fmt.Sprintf(`{"model":%q}`, model))
				if w.Code != status || calls != 1 || w.Header().Get("Retry-After") != "15" || w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), "test-secret") {
					t.Fatalf("unsafe/retried paid error: status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body)
				}
			})
		}
	}
}

func TestJevPaidTransportErrorsNeverRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{{"timeout", os.ErrDeadlineExceeded, 504}, {"network", errors.New("test-secret network failure"), 502}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := routedHandler(testConfig(), func(*http.Request) (*http.Response, error) {
				calls++
				return nil, tc.err
			})
			w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
			if calls != 1 || w.Code != tc.status || strings.Contains(w.Body.String(), "test-secret") {
				t.Fatalf("paid transport error: calls=%d status=%d body=%s", calls, w.Code, w.Body)
			}
		})
	}
}

func TestJevPaidHeaderTimeoutNeverRetries(t *testing.T) {
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer u.Close()
	tr := &http.Transport{ResponseHeaderTimeout: 50 * time.Millisecond}
	defer tr.CloseIdleConnections()
	h := newHandler(func() (Config, error) { return testConfig(), nil }, &http.Client{Transport: tr}, u.URL)
	w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
	if w.Code != 504 || calls.Load() != 1 {
		t.Fatalf("paid header timeout retried: status=%d calls=%d", w.Code, calls.Load())
	}
}

func TestJevPaidCallerCancellationAbortsUpstream(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer u.Close()
	h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest("POST", "http://127.0.0.1:6446/v1/systemone", strings.NewReader(`{"model":"jev-1.14"}`)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	ended := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), r)
		close(ended)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("paid inference did not start")
	}
	cancel()
	for _, event := range []<-chan struct{}{cancelled, ended} {
		select {
		case <-event:
		case <-time.After(time.Second):
			t.Fatal("paid inference ignored caller cancellation")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("caller cancellation triggered %d paid attempts", calls.Load())
	}
}

type paidTimedOutBody struct{ closed bool }

func (*paidTimedOutBody) Read([]byte) (int, error) { return 0, os.ErrDeadlineExceeded }
func (b *paidTimedOutBody) Close() error           { b.closed = true; return nil }

func TestJevPaidBodyFailureNeverRetriesAndClosesResponse(t *testing.T) {
	calls := 0
	body := &paidTimedOutBody{}
	h := routedHandler(testConfig(), func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
	})
	w := request(h, "POST", "/v1/systemone", `{"model":"jev-1.14"}`)
	if calls != 1 || w.Code != 502 || !body.closed {
		t.Fatalf("interrupted paid response: calls=%d status=%d closed=%t", calls, w.Code, body.closed)
	}
}

func TestJevPaidCatalogExposesOnlyCanonicalIDs(t *testing.T) {
	w := request(newHandler(func() (Config, error) { return testConfig(), nil }, nil, ""), "GET", "/v1/models", "")
	var catalog struct{ Data []struct{ ID string } }
	if json.Unmarshal(w.Body.Bytes(), &catalog) != nil {
		t.Fatal("invalid model catalog")
	}
	seen := map[string]bool{}
	for _, model := range catalog.Data {
		seen[model.ID] = true
	}
	if !seen["jev-1.14"] || !seen["beatapi/jev-1.14"] || seen["jev-1.14-free"] || seen["beatapi/jev-1.14-free"] {
		t.Fatalf("catalog advertises the wrong Jev billing mode: %s", w.Body)
	}
}

func TestJevgrepMigratesLegacyToPaidWithoutReplacingOriginalBackup(t *testing.T) {
	c := testCLI(t)
	p := filepath.Join(c.base, "jevgrep/credentials.json")
	if err := privateDir(filepath.Dir(p)); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"provider":"custom","apiKey":"local","baseURL":"http://127.0.0.1:6446/v1","model":"jev-1.14-free"}`)
	if err := atomicPrivate(p, legacy); err != nil {
		t.Fatal(err)
	}
	if !c.jevgrepConnected(testConfig()) {
		t.Fatal("legacy local jg is no longer recognized by doctor")
	}
	if err := privateDir(filepath.Dir(c.backupPath())); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"provider":"previous-provider","apiKey":"original-private-key"}`)
	if err := atomicPrivate(c.backupPath(), original); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JG_CREDS", p)
	fakeCommand(t, "jg", `read key
[ "$key" = local ] || exit 3
[ "$*" = 'auth --provider custom --base-url http://127.0.0.1:6446/v1 --model jev-1.14 --stdin' ] || exit 4
printf '%s' '{"provider":"custom","apiKey":"local","baseURL":"http://127.0.0.1:6446/v1","model":"jev-1.14"}' > "$JG_CREDS"
`)
	if err := c.connectJevgrep(testConfig()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), `"model":"jev-1.14"`) || !c.jevgrepConnected(testConfig()) {
		t.Fatalf("jg did not migrate to paid Jev: %s", got)
	}
	backup, _ := os.ReadFile(c.backupPath())
	if !bytes.Equal(backup, original) {
		t.Fatal("migration replaced the original credentials backup")
	}
}
