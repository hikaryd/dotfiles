package desktop

import (
	"strings"
	"testing"
)

func TestClipTransform(t *testing.T) {
	for _, tc := range []struct{ mode, in, want string }{{"compact", ` { "hello": "мир", "n": 3 } `, `{"hello":"мир","n":3}`}, {"base64", "aGVsbG8=", "hello"}, {"redact", `{"Token":"secret","nested":[{"PASSWORD":2}]}`, "[REDACTED]"}, {"time", "0", `"iso8601": "1970-01-01T00:00:00Z"`}} {
		t.Run(tc.mode, func(t *testing.T) {
			got, e := transform(tc.mode, tc.in, nil)
			if e != nil || !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, %v; want %q", got, e, tc.want)
			}
		})
	}
	for _, tc := range []struct{ mode, in string }{{"json", `{"a":1,"a":2}`}, {"json", `{"a":"\u001b"}`}, {"json", `{"a":"\ud800"}`}, {"json", "NaN"}, {"base64", "aGVsbG9="}, {"jwt", "eyJhbGciOiJub25lIn0.e30.YQ"}, {"time", "2020-01-01"}} {
		if _, e := transform(tc.mode, tc.in, nil); e == nil {
			t.Errorf("accepted invalid %s input", tc.mode)
		}
	}
}

func TestClipNumberFormattingAndJWT(t *testing.T) {
	got, e := transform("compact", `[1e0,1e6,1e16,0.0001,0.00001,-0,-0.0,123456789012345678901234567890]`, nil)
	want := `[1.0,1000000.0,1e+16,0.0001,1e-05,0,-0.0,123456789012345678901234567890]`
	if e != nil || got != want {
		t.Fatalf("number conversion %q %v", got, e)
	}
	got, e = transform("jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJmaXh0dXJlIn0._w", nil)
	if e != nil || !strings.Contains(got, `"verification": "UNVERIFIED"`) || !strings.Contains(got, `"sub": "fixture"`) {
		t.Fatalf("JWT %q %v", got, e)
	}
}

func TestRedactFullUnicodeCasefold(t *testing.T) {
	for _, pair := range [][2]string{{"Straße", "STRASSE"}, {"İ", "i\u0307"}, {"ﬃ", "FFI"}, {"ᾈ", "ἀι"}, {"և", "ԵՒ"}, {"Σ", "ς"}} {
		got, e := transform("redact", `{"`+pair[0]+`":"fixture-secret"}`, []string{pair[1]})
		if e != nil || strings.Contains(got, "fixture-secret") || !strings.Contains(got, "[REDACTED]") {
			t.Fatalf("case fold %q %q: %s %v", pair[0], pair[1], got, e)
		}
	}
	got, e := transform("redact", `{"é":"keep"}`, []string{"e\u0301"})
	if e != nil || !strings.Contains(got, "keep") {
		t.Fatal("case folding incorrectly normalized accents")
	}
}
