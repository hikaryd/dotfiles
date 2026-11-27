package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"dots/native/internal/mux"
)

type muxState struct {
	Version       int          `json:"version"`
	SavedAt       string       `json:"saved_at"`
	ActiveSession string       `json:"active_session,omitempty"`
	Sessions      []muxSession `json:"sessions"`
}

type muxSession struct {
	Name         string      `json:"name"`
	ActiveWindow int         `json:"active_window"`
	Windows      []muxWindow `json:"windows"`
}

type muxWindow struct {
	Index      int       `json:"index"`
	Name       string    `json:"name"`
	AutoRename bool      `json:"auto_rename"`
	Layout     string    `json:"layout"`
	ActivePane int       `json:"active_pane"`
	Panes      []muxPane `json:"panes"`
}

type muxPane struct {
	Index int    `json:"index"`
	Path  string `json:"path"`
}

var (
	tmuxWindowID = regexp.MustCompile(`^@[0-9]+$`)
	tmuxPaneID   = regexp.MustCompile(`^%[0-9]+$`)
)

func muxStatePath() string { return filepath.Join(dataHome(), "dots/tmux-state.json") }
func muxLockPath() string  { return filepath.Join(dataHome(), "dots/tmux-state.lock") }

func withMuxLock(nonBlocking bool, fn func(lockFile *os.File) error) (bool, error) {
	p := muxLockPath()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false, err
	}
	defer f.Close()
	how := syscall.LOCK_EX
	if nonBlocking {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		if nonBlocking && errors.Is(err, syscall.EWOULDBLOCK) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return true, fn(f)
}

func readCooldownUntil(f *os.File) int64 {
	if _, err := f.Seek(0, 0); err != nil {
		return 0
	}
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	v, _ := strconv.ParseInt(strings.TrimSpace(string(buf[:n])), 10, 64)
	return v
}

func writeCooldown(f *os.File, d time.Duration) {
	if _, err := f.Seek(0, 0); err != nil {
		return
	}
	_ = f.Truncate(0)
	_, _ = f.WriteString(strconv.FormatInt(time.Now().Add(d).UnixNano(), 10) + "\n")
	_ = f.Sync()
}

func readMuxState() (*muxState, error) {
	b, err := os.ReadFile(muxStatePath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > maxBytes {
		return nil, errors.New("Слишком большой файл состояния tmux")
	}
	var st muxState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, errors.New("Повреждённый файл состояния tmux")
	}
	return &st, nil
}

func isDefaultEmptyMain(st muxState) bool {
	if len(st.Sessions) != 1 || st.Sessions[0].Name != "main" {
		return false
	}
	s := st.Sessions[0]
	if len(s.Windows) != 1 || len(s.Windows[0].Panes) != 1 {
		return false
	}
	return resolve(s.Windows[0].Panes[0].Path) == resolve(home())
}

func captureMuxState() (muxState, error) {
	st := muxState{Version: 1, SavedAt: time.Now().UTC().Format(time.RFC3339)}
	wr, err := captureCommand(5*time.Second, 16*maxBytes, "", nil,
		mux.Argv("list-windows", "-a", "-F", "#{session_name}\t#{window_index}\t#{window_name}\t#{window_active}\t#{automatic-rename}\t#{window_layout}")...)
	if err != nil {
		return st, err
	}
	if noTmuxServer(wr) {
		return st, nil
	}
	if wr.code != 0 {
		return st, errors.New("tmux list-windows failed")
	}
	pr, err := captureCommand(5*time.Second, 16*maxBytes, "", nil,
		mux.Argv("list-panes", "-a", "-F", "#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_active}\t#{pane_current_path}")...)
	if err != nil || pr.code != 0 {
		return st, errors.New("tmux list-panes failed")
	}
	if cr, ce := captureCommand(3*time.Second, maxBytes, "", nil,
		mux.Argv("list-clients", "-F", "#{client_activity}\t#{session_name}")...); ce == nil && cr.code == 0 {
		var best int64
		for _, line := range strings.Split(strings.TrimRight(cr.out, "\n"), "\n") {
			tsStr, name, ok := strings.Cut(line, "\t")
			if !ok || name == scratchSession || name == "" {
				continue
			}
			ts, _ := strconv.ParseInt(tsStr, 10, 64)
			if ts >= best {
				best = ts
				st.ActiveSession = name
			}
		}
	}
	// После отключения последнего клиента list-clients пуст; берём последний присоединённый.
	if st.ActiveSession == "" {
		if sr, se := captureCommand(3*time.Second, maxBytes, "", nil,
			mux.Argv("list-sessions", "-F", "#{session_last_attached}\t#{session_activity}\t#{session_name}")...); se == nil && sr.code == 0 {
			var bestAtt, bestAct int64
			for _, line := range strings.Split(strings.TrimRight(sr.out, "\n"), "\n") {
				f := strings.SplitN(line, "\t", 3)
				if len(f) != 3 || f[2] == scratchSession || f[2] == "" {
					continue
				}
				att, _ := strconv.ParseInt(f[0], 10, 64)
				if att <= 0 {
					continue
				}
				act, _ := strconv.ParseInt(f[1], 10, 64)
				if st.ActiveSession == "" || att > bestAtt || (att == bestAtt && act > bestAct) {
					bestAtt, bestAct = att, act
					st.ActiveSession = f[2]
				}
			}
		}
	}

	type winKey struct {
		sess string
		idx  int
	}
	panesByWin := map[winKey][]muxPane{}
	activePaneByWin := map[winKey]int{}
	for _, line := range strings.Split(strings.TrimRight(pr.out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 5)
		if len(f) != 5 || f[0] == scratchSession {
			continue
		}
		wIdx, e1 := strconv.Atoi(f[1])
		pIdx, e2 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil {
			continue
		}
		k := winKey{f[0], wIdx}
		p := directory(f[4])
		if p == "" {
			p = home()
		}
		panesByWin[k] = append(panesByWin[k], muxPane{Index: pIdx, Path: p})
		if f[3] == "1" {
			activePaneByWin[k] = pIdx
		}
	}

	sessOrder := []string{}
	sessMap := map[string]*muxSession{}
	for _, line := range strings.Split(strings.TrimRight(wr.out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 6)
		if len(f) != 6 || f[0] == scratchSession || validateSessionName(f[0]) != nil {
			continue
		}
		wIdx, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		s, ok := sessMap[f[0]]
		if !ok {
			s = &muxSession{Name: f[0]}
			sessMap[f[0]] = s
			sessOrder = append(sessOrder, f[0])
		}
		k := winKey{f[0], wIdx}
		panes := panesByWin[k]
		sort.Slice(panes, func(i, j int) bool { return panes[i].Index < panes[j].Index })
		if len(panes) == 0 {
			panes = []muxPane{{Index: 1, Path: home()}}
		}
		s.Windows = append(s.Windows, muxWindow{
			Index:      wIdx,
			Name:       f[2],
			AutoRename: f[4] == "on" || f[4] == "1",
			Layout:     f[5],
			ActivePane: activePaneByWin[k],
			Panes:      panes,
		})
		if f[3] == "1" {
			s.ActiveWindow = wIdx
		}
	}

	for _, name := range sessOrder {
		s := sessMap[name]
		sort.Slice(s.Windows, func(i, j int) bool { return s.Windows[i].Index < s.Windows[j].Index })
		st.Sessions = append(st.Sessions, *s)
	}
	return st, nil
}

func saveMuxState(auto bool) error {
	_, err := withMuxLock(auto, func(lockFile *os.File) error {
		if auto && time.Now().UnixNano() < readCooldownUntil(lockFile) {
			return nil
		}
		st, err := captureMuxState()
		if err != nil || len(st.Sessions) == 0 {
			return err
		}
		prev, _ := readMuxState()
		if auto && isDefaultEmptyMain(st) && prev != nil && len(prev.Sessions) > 0 && !isDefaultEmptyMain(*prev) {
			return nil
		}
		if st.ActiveSession == "" && prev != nil && prev.ActiveSession != "" {
			for _, s := range st.Sessions {
				if s.Name == prev.ActiveSession {
					st.ActiveSession = prev.ActiveSession
					break
				}
			}
		}
		b, err := jsonBytes(st, true)
		if err != nil {
			return err
		}
		return atomicWrite(muxStatePath(), append(b, '\n'))
	})
	return err
}

func restoreMuxState() (int, string, error) {
	restored := 0
	activeName := ""
	_, err := withMuxLock(false, func(lockFile *os.File) error {
		st, err := readMuxState()
		if err != nil || st == nil || len(st.Sessions) == 0 {
			return err
		}
		live, err := tmuxSessions()
		if err != nil {
			return err
		}
		existing := map[string]bool{}
		for _, s := range live {
			existing[s.Name] = true
		}

		// Подавляем автосохранение хуками на время restore и короткий хвост после него.
		writeCooldown(lockFile, 10*time.Second)
		defer writeCooldown(lockFile, 3*time.Second)

		ordered := make([]muxSession, 0, len(st.Sessions))
		var active *muxSession
		for i := range st.Sessions {
			s := st.Sessions[i]
			if s.Name == st.ActiveSession {
				cp := s
				active = &cp
				continue
			}
			ordered = append(ordered, s)
		}
		if active != nil {
			ordered = append(ordered, *active)
		}

		for _, s := range ordered {
			if existing[s.Name] || validateSessionName(s.Name) != nil || len(s.Windows) == 0 {
				continue
			}
			if restored >= 64 {
				break
			}
			writeCooldown(lockFile, 10*time.Second)
			firstDir := safeDir(s.Windows[0].Panes)
			// Без -x/-y detached-сессия стартует 80x24 и роняет многопанельный split.
			out, err := command(mux.Argv("new-session", "-d", "-x", "240", "-y", "80", "-P", "-F", "#{session_id}\t#{window_id}\t#{pane_id}", "-s", s.Name, "-c", firstDir)...)
			if err != nil {
				continue
			}
			f := strings.SplitN(strings.TrimSpace(out), "\t", 3)
			if len(f) != 3 || !tmuxSessionID.MatchString(f[0]) || !tmuxWindowID.MatchString(f[1]) || !tmuxPaneID.MatchString(f[2]) {
				continue
			}
			sessID := f[0]
			activeWinID := f[1]
			restoreWindow(f[1], f[2], s.Windows[0])
			for _, w := range s.Windows[1:] {
				wDir := safeDir(w.Panes)
				wo, werr := command(mux.Argv("new-window", "-d", "-P", "-F", "#{window_id}\t#{pane_id}", "-t", sessID, "-c", wDir)...)
				if werr != nil {
					continue
				}
				wf := strings.SplitN(strings.TrimSpace(wo), "\t", 2)
				if len(wf) != 2 || !tmuxWindowID.MatchString(wf[0]) || !tmuxPaneID.MatchString(wf[1]) {
					continue
				}
				restoreWindow(wf[0], wf[1], w)
				if w.Index == s.ActiveWindow {
					activeWinID = wf[0]
				}
			}
			_, _ = captureCommand(3*time.Second, 256, "", nil, mux.Argv("select-window", "-t", activeWinID)...)
			existing[s.Name] = true
			restored++
		}
		if st.ActiveSession != "" && existing[st.ActiveSession] {
			activeName = st.ActiveSession
		}
		return nil
	})
	return restored, activeName, err
}

func safeDir(panes []muxPane) string {
	if len(panes) > 0 {
		if d := directory(panes[0].Path); d != "" {
			return d
		}
	}
	return home()
}

func restoreWindow(winID, firstPaneID string, w muxWindow) {
	paneIDs := map[int]string{}
	firstIdx := 1
	if len(w.Panes) > 0 {
		firstIdx = w.Panes[0].Index
	}
	paneIDs[firstIdx] = firstPaneID
	lastPaneID := firstPaneID
	for i := 1; i < len(w.Panes) && i < 32; i++ {
		p := w.Panes[i]
		d := directory(p.Path)
		if d == "" {
			d = home()
		}
		out, err := command(mux.Argv("split-window", "-d", "-P", "-F", "#{pane_id}", "-t", lastPaneID, "-c", d)...)
		if err != nil {
			continue
		}
		pid := strings.TrimSpace(out)
		if tmuxPaneID.MatchString(pid) {
			paneIDs[p.Index] = pid
			lastPaneID = pid
		}
		// Ребалансируем после каждого сплита, чтобы последняя панель не сжималась до нуля строк.
		_, _ = captureCommand(3*time.Second, 256, "", nil, mux.Argv("select-layout", "-t", winID, "tiled")...)
	}
	if w.Layout != "" && !strings.HasPrefix(w.Layout, "-") && !strings.ContainsFunc(w.Layout, control) {
		_, _ = captureCommand(3*time.Second, 256, "", nil, mux.Argv("select-layout", "-t", winID, "--", w.Layout)...)
	}
	if !w.AutoRename && cleanText(w.Name) {
		_, _ = captureCommand(3*time.Second, 256, "", nil, mux.Argv("rename-window", "-t", winID, "--", w.Name)...)
		_, _ = captureCommand(3*time.Second, 256, "", nil, mux.Argv("set-window-option", "-t", winID, "automatic-rename", "off")...)
	}
	if pid, ok := paneIDs[w.ActivePane]; ok {
		_, _ = captureCommand(3*time.Second, 256, "", nil, mux.Argv("select-pane", "-t", pid)...)
	}
}
