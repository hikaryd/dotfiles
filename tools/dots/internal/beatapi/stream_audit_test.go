package beatapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

func TestStreamSanitizationHandlesAllSSELineEndings(t *testing.T) {
	for _, ending := range []struct{ name, first, rest string }{{"lf", "\n", "\n"}, {"crlf", "\r\n", "\r\n"}, {"cr", "\r", "\r"}, {"mixed", "\r", "\n"}} {
		for _, bom := range []string{"", "\ufeff"} {
			for _, data := range []string{`{"error":{"message":"test-secret"}}`, `{"message":"test-secret"}`} {
				for _, fragmented := range []bool{false, true} {
					input := bom + "event: error" + ending.first + "data: " + data + ending.rest + ending.rest
					var reader io.Reader = strings.NewReader(input)
					if fragmented {
						reader = iotest.OneByteReader(reader)
					}
					w := httptest.NewRecorder()
					if err := relaySSE(w, reader); err == nil || strings.Contains(w.Body.String(), "test-secret") {
						t.Errorf("%s bom=%t fragmented=%t: error leaked: %q", ending.name, bom != "", fragmented, w.Body.String())
					}
				}
			}
		}
		input := "id: 1" + ending.first + "data: {\"ok\":true}" + ending.rest + ending.rest + "data: [DONE]" + ending.rest + ending.rest
		w := httptest.NewRecorder()
		if err := relaySSE(w, iotest.OneByteReader(strings.NewReader(input))); err != nil || !strings.Contains(w.Body.String(), "id: 1") || !strings.Contains(w.Body.String(), "[DONE]") {
			t.Errorf("%s: valid stream rejected: %v %q", ending.name, err, w.Body.String())
		}
	}
}

func TestCRDelimitedEventDispatchesBeforeUpstreamCloses(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	ended := make(chan error, 1)
	go func() {
		ended <- readSSE(r, func(event, data, raw string) error { return io.EOF })
	}()
	if _, err := io.WriteString(w, "data: {}\r\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("complete CR event waited for more upstream bytes")
	}
}

func TestInferenceAdmissionBoundKeepsHealthResponsive(t *testing.T) {
	const limit = 8
	started := make(chan struct{}, limit+1)
	release := make(chan struct{})
	var wg sync.WaitGroup
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(func() { unblock(); wg.Wait() })
	h := routedHandler(testConfig(), func(*http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-release
		return beatResponse(200, "application/json", `{}`), nil
	})
	for range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request(h, "POST", "/v1/chat/completions", `{"model":"glm-5.3-free"}`)
		}()
	}
	for range limit {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("inference slots did not start")
		}
	}
	excess := make(chan *httptest.ResponseRecorder, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		excess <- request(h, "POST", "/v1/systemone", `{"model":"jev-1.14-free"}`)
	}()
	select {
	case w := <-excess:
		if w.Code != 429 {
			t.Fatalf("excess inference was not rejected: %d", w.Code)
		}
	case <-time.After(time.Second):
		t.Fatal("excess inference reached blocked upstream")
	}
	for _, path := range []string{"/health", "/v1/models"} {
		if w := request(h, "GET", path, ""); w.Code != 200 {
			t.Fatalf("%s unavailable during inference: %d", path, w.Code)
		}
	}
	unblock()
	wg.Wait()
	if w := request(h, "POST", "/v1/chat/completions", `{"model":"glm-5.3-free"}`); w.Code != 200 {
		t.Fatalf("slot was not released: %d", w.Code)
	}
}
