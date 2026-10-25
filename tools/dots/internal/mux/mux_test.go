package mux

import (
	"reflect"
	"strings"
	"testing"
)

func TestArgvScopesRmuxWithoutReplacingTmux(t *testing.T) {
	for _, tc := range []struct {
		identity string
		want     []string
	}{
		{"", []string{"tmux", "list-panes", "-a"}},
		{"/tmp/rmux work.sock,42,1", []string{"rmux", "-S", "/tmp/rmux work.sock", "list-panes", "-a"}},
		{"/tmp/with,comma.sock,42,1", []string{"rmux", "-S", "/tmp/with,comma.sock", "list-panes", "-a"}},
		{"malformed", []string{"rmux", "-S", "malformed", "list-panes", "-a"}},
		{",42,0", []string{"rmux", "-S", ",42,0", "list-panes", "-a"}},
	} {
		t.Run(tc.identity, func(t *testing.T) {
			t.Setenv("RMUX", tc.identity)
			if got := Argv("list-panes", "-a"); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv=%q want %q", got, tc.want)
			}
			if IsRmux() != (tc.identity != "") {
				t.Fatal("wrong backend")
			}
		})
	}
}

func TestExternalEnvDoesNotSendRealTmuxToRmux(t *testing.T) {
	got := ExternalEnv([]string{"PATH=/tmp/rmux-shim-42:/opt/homebrew/bin:/usr/bin", "TMUX_PROGRAM=/tmp/rmux-shim-42/tmux", "TMUX=/wrong/socket,42,0", "RMUX=/wrong/socket,42,0", "RMUX_PANE=%1", "HERDR_ENV=1", "DOTS_SOURCE_PANE=%1", "DOTS_MUX_CLIENT=42", "HOME=/home/test"})
	want := []string{"PATH=/opt/homebrew/bin:/usr/bin", "HOME=/home/test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("external env=%s", strings.Join(got, "\n"))
	}
}

func TestSourcePaneUsesExplicitOriginThenActiveBackend(t *testing.T) {
	t.Setenv("DOTS_SOURCE_PANE", "")
	t.Setenv("RMUX", "/tmp/rmux.sock,42,0")
	t.Setenv("RMUX_PANE", "%7")
	t.Setenv("TMUX_PANE", "%99")
	t.Setenv("HERDR_ACTIVE_PANE_ID", "w1:p1")
	if got := SourcePane(); got != "%7" {
		t.Fatalf("selected stale parent pane %q", got)
	}
	t.Setenv("DOTS_SOURCE_PANE", "%3")
	if got := SourcePane(); got != "%3" {
		t.Fatalf("lost popup source pane %q", got)
	}
}
