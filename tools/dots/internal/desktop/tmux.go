package desktop

import (
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dots/native/internal/mux"
)

var tmuxSessionID = regexp.MustCompile(`^\$[0-9]+$`)

func noTmuxServer(r commandResult) bool {
	s := strings.TrimSpace(r.err)
	return r.code == 1 && (strings.HasPrefix(s, "no server running on ") || strings.HasPrefix(s, "error connecting to "))
}

func tmuxSessions() ([]session, error) {
	r, err := captureCommand(5*time.Second, 16*maxBytes, "", nil, mux.Argv("list-sessions", "-F", "#{session_id}\t#{session_windows}\t#{session_name}")...)
	if err != nil {
		return nil, err
	}
	if noTmuxServer(r) {
		return []session{}, nil
	}
	if r.code != 0 {
		return nil, errors.New("tmux session inventory failed")
	}
	panes, err := command(mux.Argv("list-panes", "-a", "-F", "#{session_id}\t#{pane_current_path}")...)
	if err != nil {
		return nil, err
	}
	paths, counts := map[string]string{}, map[string]int{}
	for _, line := range strings.Split(panes, "\n") {
		id, path, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		counts[id]++
		if paths[id] == "" && directory(path) != "" {
			paths[id] = path
		}
	}
	entries := []session{}
	for _, line := range strings.Split(strings.TrimRight(r.out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			return nil, errors.New("Некорректная сессия tmux")
		}
		windows, e := strconv.Atoi(f[1])
		if e != nil || !tmuxSessionID.MatchString(f[0]) || !cleanText(f[2]) || windows < 1 {
			return nil, errors.New("Некорректная сессия tmux")
		}
		var path *string
		if value := paths[f[0]]; value != "" {
			path = &value
		}
		panes := counts[f[0]]
		entries = append(entries, session{ID: "tmux:" + f[0], Source: "tmux", Name: f[2], Path: path, Workspace: f[0], Tabs: &windows, Panes: &panes})
	}
	return entries, nil
}

func tmuxFocus(id string) error {
	if os.Getenv("TMUX") == "" {
		if interactive(mux.Argv("attach-session", "-t", id)...) != 0 {
			return errors.New("tmux attach failed")
		}
		return nil
	}
	args := []string{"switch-client"}
	// Popup jobs have no pane of their own; target the client that opened them.
	if client := os.Getenv("DOTS_MUX_CLIENT"); client != "" {
		args = append(args, "-c", client)
	}
	_, err := command(mux.Argv(append(args, "-t", id)...)...)
	return err
}

func openEntry(entry session) error {
	if entry.Source == "tmux" {
		return tmuxFocus(entry.Workspace)
	}
	path := directory(ptrText(entry.Path))
	if path == "" {
		return errors.New("Каталог сессии больше не существует; откройте список заново")
	}
	entries, err := tmuxSessions()
	if err != nil {
		return err
	}
	matches := []session{}
	for _, existing := range entries {
		if ptrText(existing.Path) == path {
			matches = append(matches, existing)
		}
	}
	if len(matches) > 1 {
		return errors.New("Несколько сессий tmux подходят проекту; выберите tmux id")
	}
	if len(matches) == 1 {
		return tmuxFocus(matches[0].Workspace)
	}
	// Stable path-derived names avoid collisions across equal repository basenames.
	id, err := command(mux.Argv("new-session", "-d", "-P", "-F", "#{session_id}", "-s", sessionName(path), "-c", path)...)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if !tmuxSessionID.MatchString(id) {
		return errors.New("Некорректный идентификатор tmux")
	}
	return tmuxFocus(id)
}
