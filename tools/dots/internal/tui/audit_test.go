package tui

import (
	"strings"
	"testing"
)

func TestUntrustedTerminalControls(t *testing.T) {
	for _, attack := range []string{"\x1b[2J", "\x1b]52;c;WA==\a", "\x1b]0;spoof\x1b\\", "\x1bPpayload\x1b\\", "\u009b2J", "\x1b]52;c;unfinished", "\x1b[8m"} {
		row := "safe" + attack
		got := table([]string{row + "\tvalue"})[0]
		if got != "safe  value" {
			t.Errorf("unsafe display %q => %q", row, got)
		}
	}
	if got := Clean("a\x1b]52;c;WA==\ab"); got != "ab" {
		t.Errorf("OSC payload retained: %q", got)
	}
	frame := Frame{Width: 80, Color: true, Body: Green + "safe" + Reset + "\x1b[2J\x1b]52;c;WA==\a"}
	got := frame.Render()
	if strings.Contains(got, "\x1b[2J") || strings.Contains(got, "52;") {
		t.Errorf("unsafe frame: %q", got)
	}
	if !strings.Contains(got, Green+"safe"+Reset) {
		t.Fatal("trusted styling lost")
	}
}
