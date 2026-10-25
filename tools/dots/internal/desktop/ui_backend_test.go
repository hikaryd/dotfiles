package desktop

import (
	"strings"
	"testing"
)

func TestInteractiveSelectionRequiresTerminalNotExternalPickers(t *testing.T) {
	for _, args := range [][]string{{"menu"}, {"clip"}} {
		f := newFixture(t)
		out, diagnostic, code := f.run("", args...)
		if code == 0 || out != "" || !strings.Contains(diagnostic, "terminal") || len(f.calls()) != 0 {
			t.Fatalf("%v: %d %q %q %+v", args, code, out, diagnostic, f.calls())
		}
	}
}
