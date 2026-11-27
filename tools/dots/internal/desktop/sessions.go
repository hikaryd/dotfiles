package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"dots/native/internal/tui"
)

type session struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Name      string  `json:"name"`
	Path      *string `json:"path"`
	Workspace string  `json:"workspace_id,omitempty"`
	Tabs      *int    `json:"tabs,omitempty"`
	Panes     *int    `json:"panes,omitempty"`
	Fresh     bool    `json:"fresh_shells"`
	ImportKey *string `json:"import_key"`
}

func directory(s string) string {
	if !cleanText(s) {
		return ""
	}
	p := resolve(s)
	if isDir(p) {
		return p
	}
	return ""
}
func sessionKey(source, name, path string) string {
	s := source + "\x00" + name
	if source != "tmux" {
		s += "\x00" + path
	}
	return hashID(s, 16)
}
func sessionInventory() ([]session, error) {
	entries, e := tmuxSessions()
	if e != nil || !available("sesh") {
		return entries, e
	}
	raw, e := command("sesh", "list", "-j")
	var rows []any
	if e != nil || json.Unmarshal([]byte(raw), &rows) != nil || rows == nil {
		fmt.Fprintln(os.Stderr, "sesh недоступен; показаны живые сессии tmux.")
		return entries, nil
	}
	seen := map[string]bool{}
	for _, item := range rows {
		r := object(item)
		if r == nil {
			continue
		}
		source, name := str(r["Src"]), str(r["Name"])
		path := directory(str(r["Path"]))
		if !has(source, "config", "zoxide") || !cleanText(name) || path == "" {
			continue
		}
		key := sessionKey(source, name, path)
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, session{ID: "sesh:" + source + ":" + key, Source: source, Name: name, Path: &path, Fresh: true, ImportKey: &key})
	}
	return entries, nil
}

// Проекты ищем напрямую, а не по истории zoxide. Скан неглубокий: кэши,
// файлы и вложенные репозитории сессиями не считаются.
func devProjects() ([]session, error) {
	root := filepath.Join(home(), "Documents", "dev")
	rows, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("Не удалось прочитать ~/Documents/dev")
	}
	projects := []session{}
	for _, row := range rows {
		name := row.Name()
		if strings.HasPrefix(name, ".") || !cleanText(name) {
			continue
		}
		path := directory(filepath.Join(root, name))
		if path == "" {
			continue
		}
		key := sessionKey("project", name, path)
		projects = append(projects, session{ID: "project:" + key, Source: "project", Name: name, Path: &path, Fresh: true, ImportKey: &key})
	}
	return projects, nil
}

func ptrText(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func sessionsRun(args []string) int {
	if len(args) == 0 {
		args = []string{"open"}
	}
	o, e := parse(args[1:], "--json --all --auto --quiet --print-active", "")
	if e != nil || !has(args[0], "open", "list", "save", "restore") {
		return usageError("sessions")
	}
	switch args[0] {
	case "save":
		if len(o.pos) > 0 || o.flag("--json") || o.flag("--all") || o.flag("--print-active") {
			return usageError("sessions")
		}
		if err := saveMuxState(o.flag("--auto")); err != nil {
			if o.flag("--quiet") {
				return 1
			}
			return fail("dots sessions save", err)
		}
		if !o.flag("--quiet") {
			fmt.Println("Sessions saved.")
		}
		return 0
	case "restore":
		if len(o.pos) > 0 || o.flag("--json") || o.flag("--all") || o.flag("--auto") {
			return usageError("sessions")
		}
		n, active, err := restoreMuxState()
		if err != nil {
			if o.flag("--quiet") || o.flag("--print-active") {
				return 1
			}
			return fail("dots sessions restore", err)
		}
		if o.flag("--print-active") {
			if active != "" {
				fmt.Println(active)
			}
			return 0
		}
		if !o.flag("--quiet") {
			fmt.Printf("Restored sessions: %d\n", n)
		}
		return 0
	case "list":
		if len(o.pos) > 0 || o.flag("--all") || o.flag("--auto") || o.flag("--quiet") || o.flag("--print-active") {
			return usageError("sessions")
		}
		entries, err := sessionInventory()
		if err != nil {
			return fail("dots sessions", err)
		}
		if o.flag("--json") {
			return jsonExit(entries, true)
		}
		for _, s := range entries {
			suffix := ""
			if s.Fresh {
				suffix = " · fresh shells"
			}
			fmt.Printf("%s  %s [%s%s]\n", s.ID, s.Name, s.Source, suffix)
		}
		return 0
	}
	if len(o.pos) > 1 || o.flag("--json") || o.flag("--auto") || o.flag("--quiet") || o.flag("--print-active") {
		return usageError("sessions")
	}
	if len(o.pos) > 0 {
		entries, err := sessionInventory()
		if err != nil {
			return fail("dots sessions", err)
		}
		projects, err := devProjects()
		if err != nil {
			return fail("dots sessions", err)
		}
		entries = append(entries, projects...)
		matches := []int{}
		for i, s := range entries {
			if has(o.pos[0], s.ID, s.Name, ptrText(s.Path)) {
				matches = append(matches, i)
			}
		}
		if len(matches) > 1 {
			live := []int{}
			for _, i := range matches {
				if entries[i].Source == "tmux" {
					live = append(live, i)
				}
			}
			if len(live) == 1 {
				matches = live
			} else if len(live) == 0 {
				for _, i := range matches {
					if entries[i].Source != "project" {
						continue
					}
					same := true
					for _, j := range matches {
						if ptrText(entries[j].Path) != ptrText(entries[i].Path) {
							same = false
						}
					}
					if same {
						matches = []int{i}
						break
					}
				}
			}
		}
		if len(matches) != 1 {
			return fail("dots sessions", errors.New("Сессия не найдена или имя неоднозначно; используйте id из list"))
		}
		if err = openEntry(entries[matches[0]]); err != nil {
			return fail("dots sessions", err)
		}
		return 0
	}
	return sessionsInteractive(o.flag("--all"))
}

func sessionsInteractive(showAll bool) int {
	endScreen := tui.HoldScreen()
	defer endScreen()
	die := func(err error) int {
		tui.ReleaseScreen()
		return fail("dots sessions", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	notice := ""
	for {
		entries, err := sessionInventory()
		if err != nil {
			return die(err)
		}
		projects, err := devProjects()
		if err != nil {
			return die(err)
		}
		entries = append(entries, projects...)
		frame := desktopFrame("Session / workspace")
		frame.Footer = "Enter открыть · Ctrl+R переименовать · Ctrl+X закрыть · Esc назад"
		if !showAll {
			live := []session{}
			for _, s := range entries {
				if s.Source == "tmux" {
					live = append(live, s)
				}
			}
			frame.Intro = "Введите имя проекта из ~/Documents/dev, чтобы открыть или создать сессию."
			for _, p := range projects {
				alreadyLive := false
				for _, s := range live {
					if s.Source != "project" && (s.Name == p.Name || ptrText(s.Path) == ptrText(p.Path)) {
						alreadyLive = true
						break
					}
				}
				if !alreadyLive {
					frame.SearchOnly = append(frame.SearchOnly, len(live))
					live = append(live, p)
				}
			}
			entries = live
		}
		if notice != "" {
			if frame.Intro != "" {
				frame.Intro = notice + "\n" + frame.Intro
			} else {
				frame.Intro = notice
			}
			notice = ""
		}
		if len(entries) == 0 {
			return die(errors.New("Нет живых сессий; используйте dots sessions open --all для импорта"))
		}
		labels := make([]string, 0, len(entries))
		for _, s := range entries {
			detail := s.Source + " · fresh shell"
			if s.Source == "tmux" {
				detail = fmt.Sprintf("tmux %s · %d tabs", s.Workspace, *s.Tabs)
			} else if s.Source == "project" {
				detail = "project · new shell"
			}
			labels = append(labels, fmt.Sprintf("%s  [%s]  %s", s.Name, detail, ptrText(s.Path)))
		}
		selected, action, err := tui.SelectKey(ctx, frame, labels, "", []string{"ctrl-r", "ctrl-x"})
		if errors.Is(err, tui.ErrCanceled) || errors.Is(err, context.Canceled) {
			return 0
		}
		if err != nil {
			return die(err)
		}
		entry := entries[selected]
		switch action {
		case "enter":
			tui.ReleaseScreen()
			if err = openEntry(entry); err != nil {
				return fail("dots sessions", err)
			}
			return 0
		case "ctrl-r":
			if entry.Source != "tmux" {
				notice = "Переименовать можно только живую сессию tmux."
				continue
			}
			inFrame := desktopFrame("Переименовать сессию")
			inFrame.Mode = "MUTATE"
			inFrame.Tone = tui.Yellow
			inFrame.Prompt = "Новое имя"
			inFrame.Intro = fmt.Sprintf("Текущее имя: %s (%s)", entry.Name, entry.Workspace)
			newName, err := tui.Input(ctx, inFrame, entry.Name)
			if errors.Is(err, tui.ErrCanceled) || errors.Is(err, context.Canceled) {
				continue
			}
			if err != nil {
				return die(err)
			}
			if strings.TrimSpace(newName) == entry.Name {
				continue
			}
			if err := tmuxRenameSession(entry.Workspace, newName); err != nil {
				notice = "Не удалось переименовать: " + err.Error()
			} else {
				notice = "Переименовано: " + strings.TrimSpace(newName)
			}
		case "ctrl-x":
			if entry.Source != "tmux" {
				notice = "Закрыть можно только живую сессию tmux."
				continue
			}
			cf := desktopFrame("Закрыть сессию")
			cf.Mode = "KILL"
			cf.Tone = tui.Red
			tabs, panes := 0, 0
			if entry.Tabs != nil {
				tabs = *entry.Tabs
			}
			if entry.Panes != nil {
				panes = *entry.Panes
			}
			cf.Intro = fmt.Sprintf("Закрыть сессию %s (%s, %d вкладок, %d панелей)?", entry.Name, entry.Workspace, tabs, panes)
			if err := tui.Confirm(ctx, cf); errors.Is(err, tui.ErrCanceled) || errors.Is(err, context.Canceled) {
				continue
			} else if err != nil {
				return die(err)
			}
			if err := tmuxKillSession(entry.Workspace); err != nil {
				notice = "Не удалось закрыть: " + err.Error()
			} else {
				notice = "Сессия закрыта: " + entry.Name
			}
		}
	}
}
