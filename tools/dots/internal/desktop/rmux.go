package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"dots/native/internal/mux"
)

var rmuxSessionID = regexp.MustCompile(`^\$[0-9]+$`)

const rmuxImportOption = "@dots-session-import"

func rmuxSessions() ([]session, error) {
	raw, err := command(mux.Argv("list-sessions", "--json")...)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID      string `json:"session_id"`
		Name    string `json:"session_name"`
		Windows int    `json:"session_windows"`
	}
	if json.Unmarshal([]byte(raw), &rows) != nil || rows == nil {
		return nil, errors.New("Некорректный список rmux")
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
	for _, row := range rows {
		if !rmuxSessionID.MatchString(row.ID) || !cleanText(row.Name) || row.Windows < 1 {
			return nil, errors.New("Некорректная сессия rmux")
		}
		var path *string
		if value := paths[row.ID]; value != "" {
			path = &value
		}
		tabs, panes := row.Windows, counts[row.ID]
		entries = append(entries, session{ID: "rmux:" + row.ID, Source: "rmux", Name: row.Name, Path: path, Workspace: row.ID, Tabs: &tabs, Panes: &panes})
	}
	return entries, nil
}

func rmuxFocus(id string) error {
	args := []string{"switch-client"}
	if client := os.Getenv("DOTS_MUX_CLIENT"); client != "" {
		args = append(args, "-c", client)
	}
	_, err := command(mux.Argv(append(args, "-t", id)...)...)
	return err
}

func rmuxOpenEntry(entry session) error {
	if entry.Source == "rmux" {
		return rmuxFocus(entry.Workspace)
	}
	path := directory(ptrText(entry.Path))
	if path == "" {
		return errors.New("Каталог сессии больше не существует; откройте список заново")
	}
	entries, err := rmuxSessions()
	if err != nil {
		return err
	}
	// Stable path-derived names avoid collisions across equal repository basenames.
	name := sessionName(path)
	key := sessionKey(entry.Source, entry.Name, path)
	if entry.Source == "tmux" {
		name += "-" + key[:8]
	}
	matches := []session{}
	for _, existing := range entries {
		if entry.Source == "tmux" {
			if existing.Name != name {
				continue
			}
			state, e := command(mux.Argv("show-options", "-qv", "-t", existing.Workspace, rmuxImportOption)...)
			if e != nil {
				return e
			}
			if strings.TrimSpace(state) != key {
				return errors.New("Незавершённый импорт rmux; выберите живую сессию для восстановления")
			}
			matches = append(matches, existing)
		} else if ptrText(existing.Path) == path {
			matches = append(matches, existing)
		}
	}
	if len(matches) > 1 {
		return errors.New("Несколько сессий rmux подходят проекту; выберите rmux id")
	}
	if len(matches) == 1 {
		return rmuxFocus(matches[0].Workspace)
	}
	var layout []layoutWindow
	if entry.Source == "tmux" {
		layout, err = tmuxLayout(entry.Name)
		if err != nil {
			return err
		}
	}
	firstPath := path
	if len(layout) > 0 && layout[0].panes[0].path != "" {
		firstPath = layout[0].panes[0].path
	}
	args := []string{"new-session", "-d", "-P", "-F", "#{session_id}", "-s", name, "-c", firstPath}
	if len(layout) > 0 {
		args = append(args, "-n", layout[0].label)
	}
	id, err := command(mux.Argv(args...)...)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if !rmuxSessionID.MatchString(id) {
		return errors.New("Некорректный идентификатор rmux")
	}
	if len(layout) > 0 {
		if _, err = command(mux.Argv("set-option", "-t", id, rmuxImportOption, "pending:"+key)...); err != nil {
			return err
		}
	}
	for i, window := range layout {
		target := id + ":" + strconv.Itoa(i+1)
		cwd := path
		if window.panes[0].path != "" {
			cwd = window.panes[0].path
		}
		if i > 0 {
			if _, err = command(mux.Argv("new-window", "-d", "-t", target, "-n", window.label, "-c", cwd)...); err != nil {
				return err
			}
		}
		if _, err = command(mux.Argv("set-window-option", "-t", target, "automatic-rename", "off")...); err != nil {
			return err
		}
		for _, pane := range window.panes[1:] {
			cwd := pane.path
			if cwd == "" {
				cwd = path
			}
			if _, err = command(mux.Argv("split-window", "-d", "-h", "-t", target, "-c", cwd)...); err != nil {
				return err
			}
		}
	}
	if len(layout) > 0 {
		if _, err = command(mux.Argv("set-option", "-t", id, rmuxImportOption, key)...); err != nil {
			return err
		}
		fmt.Println("Созданы новые rmux shells; исходные процессы tmux сохранены и не переносились.")
	}
	return rmuxFocus(id)
}
