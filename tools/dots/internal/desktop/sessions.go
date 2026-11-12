package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// Project candidates are discovered directly, not inferred from zoxide history.
// This is a shallow scan: caches, files and nested repositories are not sessions.
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
	o, e := parse(args[1:], "--json --all", "")
	if e != nil || !has(args[0], "open", "list") || args[0] == "list" && (len(o.pos) > 0 || o.flag("--all")) || args[0] == "open" && (len(o.pos) > 1 || o.flag("--json")) {
		return usageError("sessions")
	}
	entries, e := sessionInventory()
	if e != nil {
		return fail("dots sessions", e)
	}
	if args[0] == "list" {
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
	projects, e := devProjects()
	if e != nil {
		return fail("dots sessions", e)
	}
	entries = append(entries, projects...)
	selected := -1
	if len(o.pos) > 0 {
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
				// A project may also have a sesh/zoxide entry for the same path.
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
		selected = matches[0]
	} else {
		frame := desktopFrame("Session / workspace")
		if !o.flag("--all") {
			// Show live sessions initially, and discover new projects on search.
			// Keep the full import/history inventory behind --all.
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
		if len(entries) == 0 {
			return fail("dots sessions", errors.New("Нет живых сессий; используйте dots sessions open --all для импорта"))
		}
		labels := []string{}
		for _, s := range entries {
			detail := s.Source + " · fresh shell"
			if s.Source == "tmux" {
				detail = fmt.Sprintf("tmux %s · %d tabs", s.Workspace, *s.Tabs)
			} else if s.Source == "project" {
				detail = "project · new shell"
			}
			labels = append(labels, fmt.Sprintf("%s  [%s]  %s", s.Name, detail, ptrText(s.Path)))
		}
		selected, e = selectFrame(labels, frame)
		if e != nil {
			return fail("dots sessions", e)
		}
	}
	if selected >= 0 {
		if e = openEntry(entries[selected]); e != nil {
			return fail("dots sessions", e)
		}
	}
	return 0
}
