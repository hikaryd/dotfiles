package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"dots/native/internal/mux"
)

type pane struct {
	Session  string `json:"session"`
	Name     string `json:"name"`
	Window   string `json:"window"`
	Pane     string `json:"pane"`
	Attached bool   `json:"attached"`
	Cwd      string `json:"cwd"`
	Command  string `json:"command"`
	Active   bool   `json:"active"`
	pid      int
}
type project struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Panes []pane `json:"panes"`
}

func registryPath() string { return filepath.Join(configHome(), "dots/projects.json") }
func allowed(path string) bool {
	p := filepath.Clean(expand(path))
	cfg := configHome()
	if !cleanText(p) || has(".omx", strings.Split(p, string(os.PathSeparator))...) || p == home() || within(p, cfg) {
		return false
	}
	p = resolve(p)
	return isDir(p) && !has(".omx", strings.Split(p, string(os.PathSeparator))...) && p != resolve(home()) && !within(p, resolve(cfg))
}
func gitRoot(path string) string {
	if !allowed(path) {
		return ""
	}
	r, e := captureCommand(5*time.Second, 65536, "", nil, "git", "-C", path, "rev-parse", "--show-toplevel")
	root := strings.TrimSpace(r.out)
	if e == nil && r.code == 0 && allowed(root) {
		return resolve(root)
	}
	return ""
}

type metadata struct{ root, common string }

func gitMetadata(path string) metadata {
	if !allowed(path) {
		return metadata{}
	}
	current := resolve(path)
	for candidate := current; ; candidate = filepath.Dir(candidate) {
		marker := filepath.Join(candidate, ".git")
		if isDir(marker) || isFile(marker) {
			if !allowed(candidate) {
				return metadata{}
			}
			gitdir := marker
			if isFile(marker) {
				b, e := os.ReadFile(marker)
				if e != nil {
					return metadata{}
				}
				line := strings.SplitN(string(b), "\n", 2)[0]
				if !strings.HasPrefix(line, "gitdir: ") {
					return metadata{}
				}
				gitdir = strings.TrimPrefix(line, "gitdir: ")
				if !filepath.IsAbs(gitdir) {
					gitdir = filepath.Join(candidate, gitdir)
				}
			}
			gitdir = resolve(gitdir)
			if !isDir(gitdir) {
				return metadata{}
			}
			common := gitdir
			if b, e := os.ReadFile(filepath.Join(gitdir, "commondir")); e == nil && strings.TrimSpace(string(b)) != "" {
				v := strings.TrimSpace(string(b))
				if filepath.IsAbs(v) {
					common = resolve(v)
				} else {
					common = resolve(filepath.Join(gitdir, v))
				}
			}
			if isFile(filepath.Join(gitdir, "HEAD")) && isDir(filepath.Join(common, "objects")) {
				if within(current, gitdir) {
					return metadata{}
				}
				return metadata{resolve(candidate), common}
			}
		}
		if filepath.Dir(candidate) == candidate {
			break
		}
	}
	return metadata{}
}
func registeredWorktrees(common string) []string {
	paths := []string{}
	if filepath.Base(common) == ".git" {
		paths = append(paths, filepath.Dir(common))
	}
	entries, _ := os.ReadDir(filepath.Join(common, "worktrees"))
	for _, entry := range entries {
		b, e := os.ReadFile(filepath.Join(common, "worktrees", entry.Name(), "gitdir"))
		if e == nil {
			p := strings.TrimSpace(string(b))
			if filepath.IsAbs(p) {
				paths = append(paths, filepath.Dir(p))
			}
		}
	}
	return paths
}
func hashID(s string, n int) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:n]
}
func projectID(path string) string { return hashID(path, 12) }
func sessionName(path string) string {
	slug := strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(filepath.Base(path), "-"), "-")
	if slug == "" {
		slug = "project"
	}
	return truncate(slug, 45) + "-" + projectID(path)
}
func livePanes() ([]pane, error) { return livePanesContext(context.Background()) }
func livePanesContext(ctx context.Context) ([]pane, error) {
	panes := []pane{}
	if inHerdr() {
		s, e := herdrSnapshotContext(ctx)
		if e != nil {
			return nil, e
		}
		if array(s["panes"]) == nil {
			return nil, errors.New("Некорректный снимок рабочих пространств Herdr")
		}
		workspaces := map[string]map[string]any{}
		for _, v := range array(s["workspaces"]) {
			w := object(v)
			if id, ok := w["workspace_id"].(string); ok {
				workspaces[id] = w
			}
		}
		for _, v := range array(s["panes"]) {
			p := object(v)
			id := str(p["workspace_id"])
			w := workspaces[id]
			cwd, ok := p["cwd"].(string)
			if w == nil || !ok {
				continue
			}
			active, _ := p["focused"].(bool)
			panes = append(panes, pane{id, str(w["label"]), str(p["tab_id"]), str(p["pane_id"]), true, cwd, "herdr", active, 0})
		}
		return panes, nil
	}
	r, e := captureCommandContext(ctx, 5*time.Second, 16*maxBytes, "", nil, mux.Argv("list-panes", "-a", "-F", "#{session_id}\t#{session_name}\t#{window_id}\t#{pane_id}\t#{session_attached}\t#{pane_current_command}\t#{pane_active}\t#{pane_pid}\t#{pane_current_path}")...)
	if e != nil {
		return nil, e
	}
	if r.code != 0 {
		if r.code == 1 && strings.HasPrefix(strings.TrimSpace(r.err), "no server running on ") {
			return panes, nil
		}
		return nil, errors.New("tmux pane inventory failed")
	}
	for _, line := range strings.Split(r.out, "\n") {
		f := strings.SplitN(line, "\t", 9)
		if len(f) == 9 {
			pid, err := strconv.Atoi(f[7])
			if err != nil || pid <= 1 {
				pid = 0
			}
			panes = append(panes, pane{f[0], f[1], f[2], f[3], f[4] != "0", f[8], f[5], f[6] == "1", pid})
		}
	}
	return panes, nil
}
func savedPaths() ([]string, error) {
	b, e := os.ReadFile(registryPath())
	if os.IsNotExist(e) {
		return []string{}, nil
	}
	if e != nil {
		return nil, errors.New("projects.json недоступен")
	}
	return validateRegistry(b)
}
func validateRegistry(b []byte) ([]string, error) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) != nil || string(raw["version"]) != "1" {
		return nil, errors.New("Некорректная схема registry projects.json (ожидается version=1).")
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(raw["projects"], &rows) != nil || rows == nil {
		return nil, errors.New("Некорректная схема registry projects.json (ожидается version=1).")
	}
	paths := []string{}
	for _, r := range rows {
		var id, p string
		if json.Unmarshal(r["id"], &id) != nil || id == "" || json.Unmarshal(r["path"], &p) != nil || !filepath.IsAbs(p) || !cleanText(p) {
			return nil, errors.New("Некорректная запись проекта в projects.json.")
		}
		paths = append(paths, p)
	}
	return paths, nil
}
func sortPanes(path string, panes []pane) {
	shell := func(p pane) bool {
		return has(strings.TrimLeft(filepath.Base(p.Command), "-"), ".", "", "zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh", "csh")
	}
	sort.SliceStable(panes, func(i, j int) bool {
		a, b := panes[i], panes[j]
		aa := []bool{!a.Attached, a.Name != filepath.Base(path), shell(a), !a.Active}
		bb := []bool{!b.Attached, b.Name != filepath.Base(path), shell(b), !b.Active}
		for k := range aa {
			if aa[k] != bb[k] {
				return !aa[k]
			}
		}
		if a.Session != b.Session {
			return a.Session < b.Session
		}
		return a.Pane < b.Pane
	})
}
func inventory(worktrees bool) ([]project, error) {
	return inventoryContext(context.Background(), worktrees)
}
func inventoryContext(ctx context.Context, worktrees bool) ([]project, error) {
	roots := map[string][]pane{}
	cache := map[string]metadata{}
	repositories := map[string]string{}
	rootFor := func(path string) metadata {
		m, ok := cache[path]
		if !ok {
			m = gitMetadata(path)
			cache[path] = m
			if m.root != "" {
				cache[m.root] = m
			}
		}
		if m.root != "" {
			if _, ok := repositories[m.common]; !ok {
				repositories[m.common] = m.root
			}
		}
		return m
	}
	panes, e := livePanesContext(ctx)
	if e != nil {
		return nil, e
	}
	for _, p := range panes {
		m := rootFor(p.Cwd)
		if m.root != "" {
			roots[m.root] = append(roots[m.root], p)
		}
	}
	saved, e := savedPaths()
	if e != nil {
		return nil, e
	}
	add := func(path string) {
		m := rootFor(path)
		if m.root != "" {
			if _, ok := roots[m.root]; !ok {
				roots[m.root] = []pane{}
			}
		}
	}
	for _, p := range saved {
		add(p)
	}
	commons := []string{}
	for c := range repositories {
		commons = append(commons, c)
	}
	sort.Strings(commons)
	for _, c := range commons {
		for _, p := range registeredWorktrees(c) {
			m := rootFor(p)
			if m.root != "" && m.common == c {
				add(p)
			}
		}
	}
	if worktrees {
		for _, c := range commons {
			r, e := captureCommandContext(ctx, 5*time.Second, 16*maxBytes, "", nil, "git", "-C", repositories[c], "worktree", "list", "--porcelain", "-z")
			if e != nil {
				return nil, e
			}
			if r.code != 0 {
				return nil, errors.New("git worktree inventory failed")
			}
			for _, f := range strings.Split(r.out, "\x00") {
				if strings.HasPrefix(f, "worktree ") {
					add(f[9:])
				}
			}
		}
	}
	paths := []string{}
	for p := range roots {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	result := []project{}
	for _, p := range paths {
		ps := roots[p]
		sortPanes(p, ps)
		result = append(result, project{projectID(p), filepath.Base(p), p, ps})
	}
	return result, nil
}
func saveProjects(projects []project) error {
	if _, e := savedPaths(); e != nil {
		return e
	}
	rows := []map[string]string{}
	for _, p := range projects {
		rows = append(rows, map[string]string{"id": p.ID, "path": p.Path})
	}
	b, e := jsonBytes(map[string]any{"version": 1, "projects": rows}, true)
	if e != nil {
		return e
	}
	if _, e = validateRegistry(b); e != nil {
		return e
	}
	return atomicWrite(registryPath(), append(b, '\n'))
}
func focusPane(p pane) error {
	if inHerdr() {
		return herdrFocus(p.Session)
	}
	if mux.IsRmux() {
		if _, err := command(mux.Argv("select-pane", "-t", p.Pane)...); err != nil {
			return err
		}
		if _, err := command(mux.Argv("select-window", "-t", p.Window)...); err != nil {
			return err
		}
		return rmuxFocus(p.Session)
	}
	action := "attach-session"
	if os.Getenv("TMUX") != "" {
		action = "switch-client"
	}
	if interactive("tmux", "select-window", "-t", p.Window, ";", "select-pane", "-t", p.Pane, ";", action, "-t", p.Session) != 0 {
		return errors.New("tmux focus failed")
	}
	return nil
}
func openProject(target string) error {
	projects, e := inventory(false)
	if e != nil {
		return e
	}
	var selected *project
	for i, p := range projects {
		if target != "" && has(target, p.ID, p.Path) {
			selected = &projects[i]
			break
		}
	}
	if target == "" {
		lines := []string{}
		for _, p := range projects {
			dot := "○"
			if len(p.Panes) > 0 {
				dot = "●"
			}
			lines = append(lines, fmt.Sprintf("%s  %s %s  %s", p.ID, dot, p.Name, p.Path))
		}
		if len(lines) == 0 {
			return errors.New("Нет проектов: откройте Git-проект в tmux или передайте путь.")
		}
		i, e := selectLine(lines, "Проект / worktree")
		if e != nil {
			return e
		}
		if i < 0 {
			return nil
		}
		selected = &projects[i]
	} else if selected == nil {
		root := gitRoot(target)
		if root == "" {
			return errors.New("Не найден Git-проект")
		}
		selected = &project{Path: root}
		for i, p := range projects {
			if p.Path == root {
				selected = &projects[i]
				break
			}
		}
	}
	if gitRoot(selected.Path) != selected.Path {
		return errors.New("Выбранный Git-проект изменился; откройте список заново.")
	}
	live, e := livePanes()
	if e != nil {
		return e
	}
	panes := []pane{}
	for _, p := range live {
		if gitMetadata(p.Cwd).root == selected.Path {
			panes = append(panes, p)
		}
	}
	sortPanes(selected.Path, panes)
	if len(panes) > 0 {
		return focusPane(panes[0])
	}
	path := selected.Path
	if inHerdr() {
		r, e := herdrCreate(path, filepath.Base(path), "")
		if e != nil {
			return e
		}
		workspace, e := responseID(r, "workspace", "workspace_id")
		if e != nil {
			return e
		}
		tab, e := responseID(r, "tab", "tab_id")
		if e != nil {
			return e
		}
		paneID, e := responseID(r, "root_pane", "pane_id")
		if e != nil {
			return e
		}
		if e = herdrRename(tab, "editor"); e != nil {
			return e
		}
		for _, label := range []string{"shell", "logs"} {
			if _, e = herdrCreate(path, label, workspace); e != nil {
				return e
			}
		}
		if available("nvim") {
			if _, e = command("herdr", "pane", "run", paneID, "nvim"); e != nil {
				return e
			}
		}
		if e = herdrFocus(workspace); e != nil {
			return e
		}
		_, e = herdrCall("tab.focus", map[string]any{"tab_id": tab}, "tab", "focus", tab)
		return e
	}
	argv := mux.Argv("new-session", "-d", "-P", "-F", "#{session_id}\t#{window_id}\t#{pane_id}", "-s", sessionName(path), "-n", "editor", "-c", path)
	if available("nvim") {
		argv = append(argv, "nvim")
	}
	s, e := command(argv...)
	if e != nil {
		return e
	}
	f := strings.Split(strings.TrimSpace(s), "\t")
	if len(f) != 3 {
		return errors.New("Некорректный ответ tmux")
	}
	for _, label := range []string{"shell", "logs"} {
		if _, e = command(mux.Argv("new-window", "-d", "-t", f[0]+":", "-n", label, "-c", path)...); e != nil {
			return e
		}
	}
	return focusPane(pane{Session: f[0], Window: f[1], Pane: f[2]})
}
func projectsRun(args []string) int {
	if len(args) == 0 {
		args = []string{"open"}
	}
	o, e := parse(args[1:], "--json", "")
	if e != nil {
		return usageError("projects")
	}
	switch args[0] {
	case "open":
		if len(o.pos) > 1 || o.flag("--json") {
			return usageError("projects")
		}
		target := ""
		if len(o.pos) > 0 {
			target = o.pos[0]
		}
		if e = openProject(target); e != nil {
			return fail("dots projects", e)
		}
		return 0
	case "list", "refresh":
		if len(o.pos) > 0 || args[0] == "refresh" && o.flag("--json") {
			return usageError("projects")
		}
		projects, e := inventory(args[0] == "refresh")
		if e != nil {
			return fail("dots projects", e)
		}
		if args[0] == "refresh" {
			if e = saveProjects(projects); e != nil {
				return fail("dots projects", errors.New("не удалось сохранить inventory"))
			}
			fmt.Printf("%d проектов: %s\n", len(projects), registryPath())
		} else if o.flag("--json") {
			return jsonExit(projects, true)
		} else {
			for _, p := range projects {
				dot := "○"
				if len(p.Panes) > 0 {
					dot = "●"
				}
				fmt.Printf("%s  %s %s\n", p.ID, dot, p.Path)
			}
		}
		return 0
	}
	return usageError("projects")
}
