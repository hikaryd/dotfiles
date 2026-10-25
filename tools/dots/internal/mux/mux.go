// Package mux selects the active tmux-compatible backend without changing PATH.
package mux

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func IsRmux() bool { return os.Getenv("RMUX") != "" }

// Socket strips only the two numeric suffixes, preserving commas in a path.
// Invalid identities remain explicit paths rather than falling back to a
// different user's default server.
func Socket() string {
	value := os.Getenv("RMUX")
	last := strings.LastIndexByte(value, ',')
	if last < 0 {
		return value
	}
	previous := strings.LastIndexByte(value[:last], ',')
	if previous <= 0 {
		return value
	}
	_, pidErr := strconv.ParseUint(value[previous+1:last], 10, 64)
	_, sessionErr := strconv.ParseUint(value[last+1:], 10, 64)
	if pidErr != nil || sessionErr != nil {
		return value
	}
	return value[:previous]
}

// ExternalEnv is for inventory of the original tmux, never for active rmux
// actions. rmux injects a tmux shim and TMUX socket into every child process.
func ExternalEnv(env []string) []string {
	out := []string{}
	for _, item := range env {
		key, value, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "HERDR_") || strings.HasPrefix(key, "TMUX") || strings.HasPrefix(key, "RMUX") || key == "DOTS_SOURCE_PANE" || key == "DOTS_MUX_CLIENT" {
			continue
		}
		if key == "PATH" {
			paths := []string{}
			for _, dir := range filepath.SplitList(value) {
				if !strings.HasPrefix(filepath.Base(dir), "rmux-shim-") {
					paths = append(paths, dir)
				}
			}
			item = "PATH=" + strings.Join(paths, string(os.PathListSeparator))
		}
		out = append(out, item)
	}
	return out
}

func Argv(args ...string) []string {
	argv := []string{"tmux"}
	if IsRmux() {
		argv = []string{"rmux", "-S", Socket()}
	}
	return append(argv, args...)
}

func SourcePane() string {
	if pane := os.Getenv("DOTS_SOURCE_PANE"); pane != "" {
		return pane
	}
	if IsRmux() {
		return os.Getenv("RMUX_PANE")
	}
	for _, name := range []string{"HERDR_ACTIVE_PANE_ID", "HERDR_PANE_ID", "TMUX_PANE"} {
		if pane := os.Getenv(name); pane != "" {
			return pane
		}
	}
	return ""
}
