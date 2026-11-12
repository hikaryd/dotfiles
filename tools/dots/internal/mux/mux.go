package mux

import "os"

func Argv(args ...string) []string { return append([]string{"tmux"}, args...) }

// Popups have no pane of their own, so bindings pass the pane that opened them.
func SourcePane() string {
	if pane := os.Getenv("DOTS_SOURCE_PANE"); pane != "" {
		return pane
	}
	return os.Getenv("TMUX_PANE")
}
