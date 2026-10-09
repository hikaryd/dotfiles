package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func notifyCapture(args ...string) string {
	r, e := captureCommand(3*time.Second, maxBytes, "", nil, args...)
	if e != nil || r.code != 0 {
		return ""
	}
	return strings.TrimSpace(r.out)
}
func frontmostGhostty() bool {
	return notifyCapture("osascript", "-l", "JavaScript", "-e", `ObjC.import("AppKit"); $.NSWorkspace.sharedWorkspace.frontmostApplication.bundleIdentifier.js`) == "com.mitchellh.ghostty"
}
func safeLabel(s string) string {
	return truncate(strings.Map(func(r rune) rune {
		if r < 32 || r >= 127 && r <= 159 || r == ';' {
			return -1
		}
		return r
	}, s), 64)
}
func notification(status int, seconds float64, project string) []byte {
	title := "dots · OK"
	if status != 0 {
		title = fmt.Sprintf("dots · ошибка %d", status)
	}
	body := fmt.Sprintf("Команда завершена · %.0fs", seconds)
	if project != "" {
		body = fmt.Sprintf("%s · %.0fs", safeLabel(project), seconds)
	}
	return []byte("\x1b]777;notify;" + title + ";" + body + "\a")
}
func writeTerminal(path string, payload []byte, owner bool) bool {
	f, e := openNoFollow(path, syscall.O_WRONLY|syscall.O_NOCTTY, 0)
	if e != nil {
		return false
	}
	defer f.Close()
	if owner {
		s, e := f.Stat()
		if e != nil || s.Mode()&os.ModeCharDevice == 0 {
			return false
		}
		st, ok := s.Sys().(*syscall.Stat_t)
		if !ok || int(st.Uid) != os.Getuid() {
			return false
		}
	} else if !isTerminal(f) {
		return false
	}
	n, e := f.Write(payload)
	return e == nil && n == len(payload)
}
func deliverNotification(payload []byte, pane string, force bool) string {
	if os.Getenv("TMUX") == "" {
		if !force && frontmostGhostty() {
			return "focused"
		}
		if writeTerminal("/dev/tty", payload, false) {
			return "sent"
		}
		return "no-terminal"
	}
	rows := notifyCapture("tmux", "list-clients", "-F", "#{client_tty}\t#{client_termname}\t#{pane_id}")
	clients := [][]string{}
	for _, line := range strings.Split(rows, "\n") {
		r := strings.Split(line, "\t")
		if len(r) == 3 && r[1] == "xterm-ghostty" {
			clients = append(clients, r)
		}
	}
	if len(clients) == 0 {
		return "no-ghostty-client"
	}
	if !force {
		for _, r := range clients {
			if r[2] == pane && frontmostGhostty() {
				return "focused"
			}
		}
	}
	for _, r := range clients {
		if regexp.MustCompile(`^/dev/tty[s0-9]+$`).MatchString(r[0]) && writeTerminal(r[0], payload, true) {
			return "sent"
		}
	}
	return "no-writable-client"
}
func notifyRun(args []string) int {
	o, e := parse(args, "--test", "--status --seconds --pane")
	if e != nil || len(o.pos) > 0 {
		return usageError("notify")
	}
	status := 0
	seconds := 0.0
	if s, ok := o.values["--status"]; ok {
		status, e = strconv.Atoi(s)
		if e != nil {
			return usageError("notify")
		}
	}
	if s, ok := o.values["--seconds"]; ok {
		seconds, e = strconv.ParseFloat(s, 64)
		if e != nil {
			return usageError("notify")
		}
	}
	if status < 0 || status > 255 || seconds < 0 || seconds >= 1e9 || math.IsNaN(seconds) {
		return usageError("notify")
	}
	if !o.flag("--test") && seconds < 30 {
		return 0
	}
	project := ""
	if readStyle() != "presentation" {
		cwd, e := os.Getwd()
		if e != nil {
			return fail("dots notify", errors.New("working directory unavailable"))
		}
		project = filepath.Base(cwd)
	}
	pane := os.Getenv("TMUX_PANE")
	if p, ok := o.values["--pane"]; ok {
		pane = p
	}
	result := deliverNotification(notification(status, seconds, project), pane, o.flag("--test"))
	state := filepath.Join(envOr("XDG_STATE_HOME", filepath.Join(home(), ".local/state")), "dots")
	if e = os.MkdirAll(state, 0700); e != nil {
		return fail("dots notify", errors.New("notification state unavailable"))
	}
	f, e := openNoFollow(filepath.Join(state, "last-notification.json"), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	if e != nil {
		return fail("dots notify", errors.New("notification state unavailable"))
	}
	if e = f.Chmod(0600); e == nil {
		e = json.NewEncoder(f).Encode(map[string]any{"result": result, "exit_code": status, "seconds": seconds, "at": time.Now().Unix()})
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return fail("dots notify", errors.New("notification state write failed"))
	}
	if o.flag("--test") {
		fmt.Printf("notification transport: %s; отображение зависит от разрешений macOS/Focus\n", result)
	}
	if has(result, "sent", "focused") {
		return 0
	}
	return 1
}
