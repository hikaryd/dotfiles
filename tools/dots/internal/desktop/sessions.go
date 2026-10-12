package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
func liveWorkspaces(snapshot map[string]any) []session {
	entries := []session{}
	for _, v := range array(snapshot["workspaces"]) {
		w := object(v)
		id, name := str(w["workspace_id"]), str(w["label"])
		if !cleanText(id) || !cleanText(name) {
			continue
		}
		var cwd *string
		for _, v := range array(snapshot["panes"]) {
			p := object(v)
			s := str(p["cwd"])
			if str(p["workspace_id"]) == id && directory(s) != "" {
				cwd = &s
				break
			}
		}
		tabs, panes := number(w["tab_count"]), number(w["pane_count"])
		var key *string
		if s, ok := object(w["tokens"])["dots_session_id"].(string); ok {
			key = &s
		}
		entries = append(entries, session{"herdr:" + id, "herdr", name, cwd, id, &tabs, &panes, false, key})
	}
	return entries
}
func sessionInventory() ([]session, error) {
	s, e := herdrSnapshot()
	if e != nil {
		return nil, e
	}
	entries := liveWorkspaces(s)
	raw, e := command("sesh", "list", "-j")
	if e != nil {
		return nil, e
	}
	var rows []any
	if json.Unmarshal([]byte(raw), &rows) != nil || rows == nil {
		return nil, errors.New("Некорректный список sesh")
	}
	seen := map[string]bool{}
	for _, item := range rows {
		r := object(item)
		if r == nil {
			continue
		}
		source, name := str(r["Src"]), str(r["Name"])
		path := directory(str(r["Path"]))
		if !has(source, "tmux", "config", "zoxide") || !cleanText(name) || path == "" {
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

type layoutPane struct {
	index int
	path  string
}
type layoutWindow struct {
	id, label string
	index     int
	panes     []layoutPane
}

func tmuxLayout(name string) ([]layoutWindow, error) {
	raw, e := command("tmux", "list-windows", "-a", "-F", "#{session_name}\t#{window_id}\t#{window_index}\t#{window_name}")
	if e != nil {
		return nil, e
	}
	windows := []layoutWindow{}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 || f[0] != name || !cleanText(f[3]) {
			continue
		}
		n, e := strconv.Atoi(f[2])
		if e != nil {
			return nil, errors.New("Некорректный индекс tmux")
		}
		windows = append(windows, layoutWindow{id: f[1], label: f[3], index: n})
	}
	raw, e = command("tmux", "list-panes", "-a", "-F", "#{session_name}\t#{window_id}\t#{pane_index}\t#{pane_current_path}")
	if e != nil {
		return nil, e
	}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 || f[0] != name {
			continue
		}
		for i := range windows {
			if windows[i].id == f[1] {
				n, e := strconv.Atoi(f[2])
				if e != nil {
					return nil, errors.New("Некорректный индекс tmux")
				}
				windows[i].panes = append(windows[i].panes, layoutPane{n, directory(f[3])})
			}
		}
	}
	sort.SliceStable(windows, func(i, j int) bool { return windows[i].index < windows[j].index })
	if len(windows) == 0 {
		return nil, errors.New("tmux-сессия изменилась; откройте список заново")
	}
	for i := range windows {
		if len(windows[i].panes) == 0 {
			return nil, errors.New("tmux-сессия изменилась; откройте список заново")
		}
		sort.SliceStable(windows[i].panes, func(a, b int) bool { return windows[i].panes[a].index < windows[i].panes[b].index })
	}
	return windows, nil
}
func ptrText(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func existingWorkspace(entry session, snapshot map[string]any, windows []layoutWindow) (string, error) {
	workspaces := liveWorkspaces(snapshot)
	matches := []session{}
	for _, w := range workspaces {
		if w.ImportKey != nil && entry.ImportKey != nil && *w.ImportKey == *entry.ImportKey {
			matches = append(matches, w)
		}
	}
	if len(matches) == 0 {
		for _, w := range workspaces {
			if w.Name == entry.Name && ptrText(w.Path) == ptrText(entry.Path) {
				matches = append(matches, w)
			}
		}
	}
	if len(matches) == 0 && entry.Source == "project" {
		for _, w := range workspaces {
			if w.Name == entry.Name || ptrText(w.Path) == ptrText(entry.Path) {
				matches = append(matches, w)
			}
		}
	}
	if len(matches) == 0 && len(windows) > 0 {
		for _, w := range workspaces {
			if w.Name != entry.Name {
				continue
			}
			tabs := []map[string]any{}
			for _, v := range array(snapshot["tabs"]) {
				tab := object(v)
				if str(tab["workspace_id"]) == w.Workspace {
					tabs = append(tabs, tab)
				}
			}
			sort.SliceStable(tabs, func(i, j int) bool { return number(tabs[i]["number"]) < number(tabs[j]["number"]) })
			same := len(tabs) == len(windows)
			if same {
				for i, tab := range tabs {
					if str(tab["label"]) != windows[i].label || number(tab["pane_count"]) != len(windows[i].panes) {
						same = false
						break
					}
				}
			}
			if same {
				matches = append(matches, w)
			}
		}
	}
	if len(matches) > 1 {
		return "", errors.New("Несколько рабочих пространств подходят сессии; выберите Herdr id")
	}
	if len(matches) == 1 {
		return matches[0].Workspace, nil
	}
	return "", nil
}
func openEntry(entry session) error {
	if entry.Source == "herdr" {
		return herdrFocus(entry.Workspace)
	}
	var windows []layoutWindow
	var e error
	if entry.Source == "tmux" {
		windows, e = tmuxLayout(entry.Name)
		if e != nil {
			return e
		}
	}
	s, e := herdrSnapshot()
	if e != nil {
		return e
	}
	workspace, e := existingWorkspace(entry, s, windows)
	if e != nil {
		return e
	}
	if workspace != "" {
		return herdrFocus(workspace)
	}
	path := directory(ptrText(entry.Path))
	if path == "" {
		return errors.New("Каталог сессии больше не существует; откройте список заново")
	}
	firstPath := path
	if len(windows) > 0 && windows[0].panes[0].path != "" {
		firstPath = windows[0].panes[0].path
	}
	created, e := herdrCreate(firstPath, entry.Name, "")
	if e != nil {
		return e
	}
	workspace, e = responseID(created, "workspace", "workspace_id")
	if e != nil {
		return e
	}
	_, e = herdrCall("workspace.report_metadata", map[string]any{"workspace_id": workspace, "source": "dots-sessions", "tokens": map[string]string{"dots_session_id": ptrText(entry.ImportKey)}}, "workspace", "report-metadata", workspace, "--source", "dots-sessions", "--token", "dots_session_id="+ptrText(entry.ImportKey))
	if e != nil {
		return e
	}
	if len(windows) > 0 {
		firstTab, e := responseID(created, "tab", "tab_id")
		if e != nil {
			return e
		}
		firstPane, e := responseID(created, "root_pane", "pane_id")
		if e != nil {
			return e
		}
		for i, w := range windows {
			rootPane := firstPane
			if i == 0 {
				if e = herdrRename(firstTab, w.label); e != nil {
					return e
				}
			} else {
				p := w.panes[0].path
				if p == "" {
					p = path
				}
				tab, e := herdrCreate(p, w.label, workspace)
				if e != nil {
					return e
				}
				rootPane, e = responseID(tab, "root_pane", "pane_id")
				if e != nil {
					return e
				}
			}
			for _, p := range w.panes[1:] {
				cwd := p.path
				if cwd == "" {
					cwd = path
				}
				_, e = herdrCall("pane.split", map[string]any{"target_pane_id": rootPane, "direction": "right", "cwd": cwd, "focus": false}, "pane", "split", rootPane, "--direction", "right", "--cwd", cwd, "--no-focus")
				if e != nil {
					return e
				}
			}
		}
	}
	if e = herdrFocus(workspace); e != nil {
		return e
	}
	suffix := ""
	if len(windows) > 0 {
		suffix = "; процессы tmux не переносились"
	}
	fmt.Printf("%s: создано рабочее пространство Herdr с пустыми shell%s\n", entry.Name, suffix)
	return nil
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
				if entries[i].Source == "herdr" {
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
				if s.Source == "herdr" {
					live = append(live, s)
				}
			}
			if len(live) == 0 {
				for _, s := range entries {
					if s.Source == "tmux" {
						live = append(live, s)
					}
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
			if s.Source == "herdr" {
				detail = fmt.Sprintf("Herdr %s · %d tabs", s.Workspace, *s.Tabs)
			} else if s.Source == "tmux" {
				detail = "tmux · copy layout as fresh shells"
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
