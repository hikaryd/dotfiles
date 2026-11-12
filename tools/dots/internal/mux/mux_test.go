package mux

import "testing"

func TestSourcePanePrefersPopupOrigin(t *testing.T) {
	t.Setenv("DOTS_SOURCE_PANE", "")
	t.Setenv("TMUX_PANE", "%9")
	if got := SourcePane(); got != "%9" {
		t.Fatalf("pane %q", got)
	}
	t.Setenv("DOTS_SOURCE_PANE", "%3")
	if got := SourcePane(); got != "%3" {
		t.Fatalf("lost popup source pane %q", got)
	}
}
