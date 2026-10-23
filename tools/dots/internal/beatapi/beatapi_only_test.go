package beatapi

import (
	"net/http"
	"strings"
	"testing"
)

// A saved API key must only ever reach BeatAPI, even for previously overlapping IDs.
func TestOnlyBeatAPIReceivesSavedKey(t *testing.T) {
	for _, tc := range []struct{ path, body string }{
		{"/v1/systemone", `{"model":"jev-1.14-free"}`},
		{"/v1/chat/completions", `{"model":"deepseek-v4.1-flash-free","messages":[]}`},
	} {
		calls := 0
		h := routedHandler(testConfig(), func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "api.beatapi.io" || r.Header.Get("Authorization") != "Bearer test-secret" {
				t.Error("saved key routed outside BeatAPI")
			}
			for name := range r.Header {
				if strings.HasPrefix(strings.ToLower(name), "x-opencode-") {
					t.Error("obsolete provider metadata")
				}
			}
			return beatResponse(200, "application/json", `{}`), nil
		})
		w := request(h, "POST", tc.path, tc.body)
		if w.Code != 200 || calls != 1 {
			t.Fatalf("BeatAPI-only route: status=%d calls=%d", w.Code, calls)
		}
	}
}

func TestRemovedModelsNeverReachNetwork(t *testing.T) {
	calls := 0
	h := routedHandler(testConfig(), func(*http.Request) (*http.Response, error) {
		calls++
		return beatResponse(200, "application/json", `{}`), nil
	})
	for _, model := range []string{"big-pickle", "beam-free", "opencode/deepseek-v4.1-flash-free", "opencode/jev-1.13-free", "jev-1.13-free"} {
		for _, path := range []string{"/v1/systemone", "/v1/chat/completions"} {
			if w := request(h, "POST", path, `{"model":"`+model+`"}`); w.Code != 400 {
				t.Errorf("removed model accepted: %s %d", model, w.Code)
			}
		}
	}
	if calls != 0 {
		t.Fatal("removed model reached network")
	}
}

func TestProviderSelectionRemovedBeforeSecretInput(t *testing.T) {
	c := testCLI(t)
	if code := Run([]string{"key", "--provider", "opencode", "--stdin"}, strings.NewReader("not-a-beat-key\n"), c.out, c.errOut); code != 2 {
		t.Fatalf("provider selection accepted: %d", code)
	}
	if c.jevgrepConnected(testConfig()) {
		t.Fatal("invalid command configured jg")
	}
}
