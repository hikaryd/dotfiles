package desktop

import (
	"strings"
	"testing"
)

func TestMenuPrefersFZFWithSingleEscapeCancellation(t *testing.T) {
	f := newFixture(t)
	f.fake("fzf", allRule("", 130))
	f.fake("gum", allRule("", 130))
	out, diagnostic, code := f.run("", "menu")
	calls := f.calls()
	if code != 0 || out != "" || diagnostic != "" || len(calls) != 1 || calls[0].Name != "fzf" {
		t.Fatalf("menu cancellation: %d %q %q %+v", code, out, diagnostic, calls)
	}
	if !strings.Contains(strings.Join(calls[0].Args, " "), "esc:abort") {
		t.Fatal("Esc must abort rather than merely blur search")
	}
}
