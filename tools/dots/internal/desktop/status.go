package desktop

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type gitState struct {
	State     string `json:"state"`
	Branch    string `json:"branch"`
	Staged    *int   `json:"staged,omitempty"`
	Unstaged  *int   `json:"unstaged,omitempty"`
	Untracked *int   `json:"untracked,omitempty"`
	Conflicts *int   `json:"conflicts,omitempty"`
	Error     string `json:"error,omitempty"`
}

func parseGit(data string) (gitState, error) {
	staged, unstaged, untracked, conflicts := 0, 0, 0, 0
	g := gitState{State: "clean", Branch: "?", Staged: &staged, Unstaged: &unstaged, Untracked: &untracked, Conflicts: &conflicts}
	fields := strings.Split(data, "\x00")
	header := false
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "## ") {
			header = true
			branch := strings.SplitN(strings.ToValidUTF8(entry[3:], "�"), "...", 2)[0]
			branch = strings.TrimPrefix(strings.TrimPrefix(branch, "No commits yet on "), "Initial commit on ")
			g.Branch = branch
			continue
		}
		if len(entry) < 3 || entry[2] != ' ' {
			return g, errors.New("Unexpected git status record")
		}
		xy := entry[:2]
		if xy == "??" {
			untracked++
		} else if has(xy, "DD", "AU", "UD", "UA", "DU", "AA", "UU") {
			conflicts++
		} else {
			if !strings.ContainsRune(" ?!", rune(xy[0])) {
				staged++
			}
			if !strings.ContainsRune(" ?!", rune(xy[1])) {
				unstaged++
			}
		}
		if strings.ContainsAny(xy, "RC") {
			i++
			if i >= len(fields) {
				return g, errors.New("Missing rename source")
			}
		}
	}
	if !header {
		return g, errors.New("Missing git branch header")
	}
	if staged+unstaged+untracked+conflicts > 0 {
		g.State = "dirty"
	}
	return g, nil
}
func statusCommand(args ...string) (commandResult, error) {
	return captureCommand(5*time.Second, 16*maxBytes, "", envSet(os.Environ(), "GIT_OPTIONAL_LOCKS=0"), args...)
}
func gitStatus(path string) gitState {
	unknown := func(msg string) gitState { return gitState{State: "unknown", Branch: "?", Error: msg} }
	r, e := statusCommand("git", "-C", path, "config", "--null", "--get-regexp", `^filter\..*\.(clean|process)$`)
	if e != nil {
		return unknown("git unavailable/timeout")
	}
	if r.code != 0 && r.code != 1 {
		return unknown("git config unavailable")
	}
	for _, record := range strings.Split(r.out, "\x00") {
		_, value, _ := strings.Cut(record, "\n")
		if value != "" {
			r, e = statusCommand("git", "-C", path, "symbolic-ref", "--short", "-q", "HEAD")
			if e != nil {
				return unknown("git unavailable/timeout")
			}
			g := unknown("executable Git filters configured; status skipped")
			g.Branch = strings.TrimSpace(r.out)
			if g.Branch == "" {
				g.Branch = "detached"
			}
			return g
		}
	}
	r, e = statusCommand("git", "--no-optional-locks", "-C", path, "-c", "core.fsmonitor=false", "status", "--porcelain=v1", "-z", "--branch", "--untracked-files=normal", "--ignore-submodules=dirty")
	if e != nil {
		return unknown("git unavailable/timeout")
	}
	if r.code != 0 {
		return unknown("git status failed")
	}
	g, e := parseGit(r.out)
	if e != nil {
		return unknown("git unavailable/timeout")
	}
	return g
}

type process struct {
	parent  int
	command string
}

func parseProcesses(data string) map[int]process {
	m := map[int]process{}
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, e := strconv.Atoi(f[0])
		parent, pe := strconv.Atoi(f[1])
		if e == nil && pe == nil && pid >= 0 && parent >= 0 {
			rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), f[0])), f[1]))
			m[pid] = process{parent, filepath.Base(rest)}
		}
	}
	return m
}

type lsofRecord struct {
	command string
	names   []string
}

func parseLsof(data string) map[int]*lsofRecord {
	records := map[int]*lsofRecord{}
	pid := -1
	for _, field := range strings.Split(data, "\x00") {
		field = strings.TrimLeft(field, "\n")
		if field == "" {
			continue
		}
		value := strings.ToValidUTF8(field[1:], "�")
		switch field[0] {
		case 'p':
			n, e := strconv.Atoi(value)
			pid = -1
			if e == nil && n >= 0 {
				pid = n
				if records[n] == nil {
					records[n] = &lsofRecord{command: "?", names: []string{}}
				}
			}
		case 'c':
			if pid >= 0 {
				records[pid].command = value
			}
		case 'n':
			if pid >= 0 {
				records[pid].names = append(records[pid].names, value)
			}
		}
	}
	return records
}
func rootForCwd(cwd string, projects []project) string {
	candidate := resolve(cwd)
	root, id := "", ""
	for _, p := range projects {
		if within(candidate, p.Path) && len(p.Path) > len(root) {
			root, id = p.Path, p.ID
		}
	}
	return id
}
func ancestorProject(pid int, processes map[int]process, panes map[int]string) string {
	visited := map[int]bool{}
	for pid != 0 && !visited[pid] {
		if p := panes[pid]; p != "" {
			return p
		}
		visited[pid] = true
		pid = processes[pid].parent
	}
	return ""
}

type portRow struct {
	PID     int    `json:"pid"`
	Process string `json:"process"`
	Address string `json:"address"`
	Via     string `json:"via"`
}

func localPorts(projects []project) (map[string][]portRow, []portRow, []string) {
	warnings := []string{}
	attributed := map[string][]portRow{}
	unassigned := []portRow{}
	r, e := statusCommand("ps", "-axo", "pid=,ppid=,comm=")
	processes := parseProcesses(r.out)
	if e != nil || r.code != 0 {
		warnings = append(warnings, "Process ancestry unavailable")
	}
	r, e = statusCommand("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn0")
	if e != nil {
		return attributed, unassigned, append(warnings, "TCP listeners unavailable (lsof missing/timeout)")
	}
	if r.code != 0 && r.code != 1 || r.code == 1 && r.err != "" {
		return attributed, unassigned, append(warnings, "TCP listeners could not be inspected")
	}
	listeners := parseLsof(r.out)
	cwd := map[int]*lsofRecord{}
	pids := []int{}
	for pid := range listeners {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	if len(pids) > 0 {
		parts := []string{}
		for _, pid := range pids {
			parts = append(parts, strconv.Itoa(pid))
		}
		r, e = statusCommand("lsof", "-a", "-p", strings.Join(parts, ","), "-d", "cwd", "-Fpn0")
		if e != nil || r.code != 0 && r.code != 1 {
			warnings = append(warnings, "Process cwd unavailable; ancestry attribution only")
		}
		cwd = parseLsof(r.out)
	}
	paneIDs := map[string]string{}
	for _, p := range projects {
		for _, pane := range p.Panes {
			paneIDs[pane.Pane] = p.ID
		}
	}
	paneProjects := map[int]string{}
	if inHerdr() {
		for pane, id := range paneIDs {
			r, e := herdrCall("pane.process_info", map[string]any{"pane_id": pane}, "pane", "process-info", "--pane", pane)
			if e != nil {
				warnings = append(warnings, "Herdr pane process attribution unavailable")
				break
			}
			pid := number(object(r["process_info"])["shell_pid"])
			if pid > 0 {
				paneProjects[pid] = id
			}
		}
	} else {
		r, e = statusCommand("tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{pane_pid}")
		if e != nil {
			warnings = append(warnings, "Pane process attribution unavailable")
		}
		for _, line := range strings.Split(r.out, "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 2 {
				continue
			}
			pid, e := strconv.Atoi(f[1])
			if e == nil && paneIDs[f[0]] != "" {
				paneProjects[pid] = paneIDs[f[0]]
			}
		}
	}
	for _, pid := range pids {
		info := listeners[pid]
		paths := []string{}
		if cwd[pid] != nil {
			paths = cwd[pid].names
		}
		project, via := "", "pane ancestry"
		if len(paths) > 0 {
			project = rootForCwd(paths[0], projects)
		}
		if project != "" {
			via = "cwd"
		} else if len(paths) == 0 {
			project = ancestorProject(pid, processes, paneProjects)
		}
		addresses := uniqueStrings(info.names)
		sort.Strings(addresses)
		for _, address := range addresses {
			row := portRow{pid, info.command, address, via}
			if project != "" {
				attributed[project] = append(attributed[project], row)
			} else {
				row.Via = "unassigned"
				unassigned = append(unassigned, row)
			}
		}
	}
	return attributed, unassigned, warnings
}

type statusProject struct {
	project
	Git   gitState  `json:"git"`
	Ports []portRow `json:"ports"`
}
type statusReport struct {
	At         string          `json:"at"`
	Projects   []statusProject `json:"projects"`
	Known      int             `json:"known_projects"`
	Unassigned []portRow       `json:"unassigned_ports"`
	Warnings   []string        `json:"warnings"`
}

func collectStatus(all bool) (statusReport, error) {
	ps, e := inventory(false)
	if e != nil {
		return statusReport{}, e
	}
	ports, unassigned, warnings := localPorts(ps)
	report := statusReport{At: time.Now().UTC().Format("2006-01-02T15:04:05+00:00"), Projects: []statusProject{}, Known: len(ps), Unassigned: unassigned, Warnings: warnings}
	for _, p := range ps {
		if all || len(p.Panes) > 0 || len(ports[p.ID]) > 0 {
			rows := ports[p.ID]
			if rows == nil {
				rows = []portRow{}
			}
			report.Projects = append(report.Projects, statusProject{project: p, Ports: rows})
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range report.Projects {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			report.Projects[i].Git = gitStatus(report.Projects[i].Path)
		}(i)
	}
	wg.Wait()
	sort.SliceStable(report.Projects, func(i, j int) bool {
		a, b := report.Projects[i], report.Projects[j]
		if (len(a.Panes) > 0) != (len(b.Panes) > 0) {
			return len(a.Panes) > 0
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Path < b.Path
	})
	return report, nil
}
func gitSummary(g gitState) string {
	if g.State == "unknown" {
		return "? unknown"
	}
	if g.State == "clean" {
		return "clean"
	}
	return fmt.Sprintf("S%d M%d ?%d !%d", *g.Staged, *g.Unstaged, *g.Untracked, *g.Conflicts)
}
func renderStatus(r statusReport) {
	fmt.Printf("PROJECTS · %s · %d/%d shown\n", r.At, len(r.Projects), r.Known)
	fmt.Println("Opened panes / local listeners · one-shot snapshot, no network")
	fmt.Printf("%-32s %-25s %-18s %-23s LISTEN\n", "PROJECT", "BRANCH", "GIT", "PROCESSES")
	width := 120
	if n, e := strconv.Atoi(os.Getenv("COLUMNS")); e == nil && n > 0 {
		width = n
	}
	fmt.Println(strings.Repeat("─", min(width, 140)))
	for _, p := range r.Projects {
		counts := map[string]int{}
		for _, pane := range p.Panes {
			counts[pane.Command]++
		}
		cmds := []string{}
		for cmd := range counts {
			cmds = append(cmds, cmd)
		}
		sort.Strings(cmds)
		processes := []string{}
		for _, cmd := range cmds {
			processes = append(processes, fmt.Sprintf("%s×%d", printable(cmd), counts[cmd]))
		}
		proc := strings.Join(processes, ",")
		if proc == "" {
			proc = "—"
		}
		ports := []string{}
		for _, port := range p.Ports {
			ports = append(ports, port.Address)
		}
		ports = uniqueStrings(ports)
		sort.Strings(ports)
		listen := strings.Join(ports, ",")
		if listen == "" {
			listen = "—"
		}
		fmt.Printf("%-32s %-25s %-18s %-23s %s\n", truncate(printable(p.Name+" ["+p.ID[:4]+"]"), 32), truncate(printable(p.Git.Branch), 25), gitSummary(p.Git), truncate(proc, 23), printable(listen))
	}
	if len(r.Projects) == 0 {
		fmt.Println("Нет открытых проектов; --all покажет сохранённые worktree.")
	}
	fmt.Println("S staged · M unstaged · ? untracked entries · ! conflicts; unknown ≠ clean")
	fmt.Printf("Unassigned TCP listeners: %d; LISTEN не доказывает готовность сервиса.\n", len(r.Unassigned))
	for _, w := range r.Warnings {
		fmt.Println("WARN: " + w)
	}
	for _, p := range r.Projects {
		if p.Git.State == "unknown" {
			fmt.Printf("WARN: %s: %s\n", printable(p.Name), printable(p.Git.Error))
		}
	}
}
func statusRun(args []string) int {
	o, e := parse(args, "--all --json --interactive", "")
	if e != nil || len(o.pos) > 0 || o.flag("--json") && o.flag("--interactive") || o.flag("--interactive") && !isTerminal(os.Stdin) {
		return usageError("status")
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		r, e := collectStatus(o.flag("--all"))
		if e != nil {
			return fail("dots status", errors.New("сбор снимка не удался; проверьте dots projects list и локальные tools."))
		}
		if o.flag("--json") {
			return jsonExit(r, true)
		}
		if o.flag("--interactive") && isTerminal(os.Stdout) {
			fmt.Print("\x1b[2J\x1b[H")
		}
		renderStatus(r)
		if !o.flag("--interactive") {
			return 0
		}
		fmt.Print("[Enter/r] обновить · [o] открыть проект · [q] выход: ")
		line, e := reader.ReadString('\n')
		if e != nil {
			return 0
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		if choice == "o" {
			labels := []string{}
			for _, p := range r.Projects {
				labels = append(labels, fmt.Sprintf("%s  %s  %s  %s", p.ID, printable(p.Name), printable(p.Git.Branch), gitSummary(p.Git)))
			}
			if len(labels) == 0 {
				return 0
			}
			i, e := selectLine(labels, []string{"filter", "--placeholder", "Открыть проект", "--no-sort", "--limit", "1"}, []string{"--prompt", "Проект > ", "--no-multi"}, false)
			if e != nil {
				return fail("dots status", e)
			}
			if i >= 0 {
				if e = openProject(r.Projects[i].ID); e != nil {
					return fail("dots status", e)
				}
			}
			return 0
		}
		if !has(choice, "", "r") {
			return 0
		}
	}
}
