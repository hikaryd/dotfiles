package mux

import "os"

func Argv(args ...string) []string { return append([]string{"tmux"}, args...) }

// У popup нет своей панели, поэтому биндинги передают панель, открывшую его.
func SourcePane() string {
	if pane := os.Getenv("DOTS_SOURCE_PANE"); pane != "" {
		return pane
	}
	return os.Getenv("TMUX_PANE")
}
