package tui_test

import (
	"strings"
	"testing"

	"dots/native/internal/tui"
)

func TestFrameIsDomainNeutralAndCleansUntrustedLabels(t *testing.T) {
	frame := tui.Frame{Brand: "dots artifacts", Title: "Choose\x1b[2J item", Subtitle: "source: local", Targets: []string{"local\r\n"}, Mode: "READ ONLY", Width: 60, Body: "α\nβ"}
	out := frame.Render()
	for _, unwanted := range []string{"dots kube", "namespace", "cluster", "\x1b"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("domain leakage %q", out)
		}
	}
	if !strings.Contains(out, "dots artifacts") || !strings.Contains(out, "source: local") {
		t.Fatal(out)
	}
}
func TestTerminalCellLayout(t *testing.T) {
	value := "界а🙂"
	padded := tui.Pad(value, 10)
	if tui.Width(padded) != 10 || tui.Width(tui.Clip(value, 3)) > 3 {
		t.Fatalf("cell width %q", padded)
	}
}
