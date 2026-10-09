package agents

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Run dispatches the agents group of dots-tools.
func Run(root string, args []string) int { return run(args, os.Stdin, os.Stdout, os.Stderr) }
func run(args []string, in io.Reader, out, errout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errout, "usage: dots-tools agents task-titles|codex-task-title|ai")
		return 2
	}
	switch args[0] {
	case "task-titles":
		return runTitles(args[1:], in, out, errout)
	case "codex-task-title":
		return runHook(args[1:], in, out)
	case "ai":
		return runAI(args[1:], in, out, errout)
	default:
		fmt.Fprintln(errout, "unknown agents command")
		return 2
	}
}
func stateFile() (string, error) {
	socket := os.Getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return "", errors.New("HERDR_SOCKET_PATH is required to select the session")
	}
	dir, ok := os.LookupEnv("HERDR_PLUGIN_STATE_DIR")
	if !ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.New("cannot locate state directory")
		}
		dir = filepath.Join(home, ".local/state/herdr/plugins/dots.task-titles")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", errors.New("cannot create task title state directory")
	}
	digest := sha256.Sum256([]byte(socket))
	return filepath.Join(dir, fmt.Sprintf("%x.json", digest[:8])), nil
}
func lockState(path string, timeout time.Duration) (func(), error) {
	f, err := os.OpenFile(strings.TrimSuffix(path, ".json")+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("cannot open task title lock")
	}
	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, errors.New("cannot lock task title state")
		}
		if !time.Now().Before(deadline) {
			f.Close()
			return nil, errors.New("task title synchronization is busy")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
func readState(path string) (*titleState, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return nil, errors.New("cannot read task title state")
	}
	return decodeState(b)
}
func writeState(path string, st *titleState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".titles-")
	if err != nil {
		return errors.New("cannot create task title state")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return errors.New("cannot save task title state")
	}
	return nil
}
func herdrRequest(method string, params map[string]any) (json.RawMessage, error) {
	// Serialize before connect: Herdr polls readable clients at 100ms intervals.
	request, err := json.Marshal(map[string]any{"id": "dots-task-titles", "method": method, "params": params})
	if err != nil {
		return nil, errors.New("cannot encode Herdr request")
	}
	request = append(request, '\n')
	conn, err := net.DialTimeout("unix", os.Getenv("HERDR_SOCKET_PATH"), 4*time.Second)
	if err != nil {
		return nil, errors.New("Herdr connection failed")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err = conn.Write(request); err != nil {
		return nil, errors.New("Herdr request failed")
	}
	var response struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(conn, 32<<20)).Decode(&response); err != nil {
		return nil, errors.New("invalid or timed out Herdr response")
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		return nil, errors.New("Herdr API error")
	}
	if len(response.Result) == 0 {
		return nil, errors.New("invalid Herdr response")
	}
	return response.Result, nil
}
func runTitles(args []string, in io.Reader, out, errout io.Writer) int {
	fail := func(err error) int { fmt.Fprintln(errout, "Herdr task titles:", err); return 1 }
	if len(args) == 0 {
		fmt.Fprintln(errout, "usage: task-titles sync|set --pane ID --title TITLE [--session-id ID] [--agent codex|pi]")
		return 2
	}
	action := args[0]
	if action == "--help" || action == "-h" {
		fmt.Fprintln(out, "usage: task-titles sync|set --pane ID --title TITLE [--session-id ID] [--agent codex|pi]")
		return 0
	}
	if action != "set" && action != "sync" {
		return 2
	}
	flags := flag.NewFlagSet("task-titles "+action, flag.ContinueOnError)
	flags.SetOutput(errout)
	var paneID, title, session, agent string
	var stdin bool
	if action == "set" {
		flags.StringVar(&paneID, "pane", "", "")
		flags.StringVar(&title, "title", "", "")
		flags.StringVar(&session, "session-id", "", "")
		flags.StringVar(&agent, "agent", "codex", "")
		flags.BoolVar(&stdin, "stdin", false, "read JSON from stdin")
		flags.BoolVar(&stdin, "json", false, "read JSON from stdin")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return 2
	}
	titlePresent := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "title" {
			titlePresent = true
		}
	})
	if action == "set" && (stdin || len(args) == 1) {
		var p struct {
			Pane    string  `json:"pane"`
			PaneID  string  `json:"pane_id"`
			Title   *string `json:"title"`
			Session string  `json:"session_id"`
			Agent   string  `json:"agent"`
		}
		if err := json.NewDecoder(in).Decode(&p); err != nil {
			return fail(errors.New("invalid title JSON"))
		}
		paneID = p.Pane
		if paneID == "" {
			paneID = p.PaneID
		}
		session = p.Session
		if p.Agent != "" {
			agent = p.Agent
		}
		if p.Title != nil {
			title = *p.Title
			titlePresent = true
		}
	}
	if action == "set" && (paneID == "" || !titlePresent || !isAgent(agent)) {
		fmt.Fprintln(errout, "set requires --pane, --title and agent codex or pi")
		return 2
	}
	path, err := stateFile()
	if err != nil {
		return fail(err)
	}
	unlock, err := lockState(path, 4*time.Second)
	if err != nil {
		return fail(err)
	}
	defer unlock()
	st, err := readState(path)
	if err != nil {
		return fail(err)
	}
	raw, err := herdrRequest("session.snapshot", map[string]any{})
	if err != nil {
		return fail(err)
	}
	var result struct {
		Snapshot *snapshot `json:"snapshot"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Snapshot == nil || result.Snapshot.Panes == nil || result.Snapshot.Tabs == nil || result.Snapshot.Layouts == nil {
		return fail(errors.New("invalid Herdr snapshot"))
	}
	save := func() error { return writeState(path, st) }
	if action == "set" {
		if err = setTitle(*result.Snapshot, st, paneID, title, session, agent); err != nil {
			return fail(err)
		}
		if err = save(); err != nil {
			return fail(err)
		}
	}
	err = synchronize(*result.Snapshot, st, func(m string, p map[string]any) error { _, e := herdrRequest(m, p); return e }, save)
	if err != nil {
		return fail(err)
	}
	return 0
}
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_@%+=:,./-", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

const hookGuidance = "Herdr task title: after understanding the task, set a concise Russian title by running the command below with one shell-quoted title argument. Use the concrete subject/problem first, append a known ticket ID at the end if it fits; 2–6 words, at most 32 characters. Examples: 'Переоткрытие заявки · 3812844', 'НСИ · Ложное увольнение', 'СБ · Создание проверки'. Keep the established subject for follow-ups such as creating an MR, testing, or continuing work; update the title when you learn the actual problem or the user changes the subject. Generic task verbs alone do not identify a task. Use only information already in this conversation; omit personal data and secrets. This changes presentation only; a title failure must not block the user's work. Only the main agent sets this title; do not delegate it to subagents.\n"

func runHook(args []string, in io.Reader, out io.Writer) int {
	failure := func() int {
		_ = json.NewEncoder(out).Encode(map[string]string{"systemMessage": "Herdr task title hook failed; title context was not added."})
		return 0
	}
	if len(args) != 1 || args[0] != "hook" {
		return failure()
	}
	var payload map[string]any
	if json.NewDecoder(in).Decode(&payload) != nil {
		return failure()
	}
	paneID, socket := os.Getenv("HERDR_PANE_ID"), os.Getenv("HERDR_SOCKET_PATH")
	session, _ := payload["session_id"].(string)
	if paneID == "" || socket == "" || session == "" || payload["hook_event_name"] != "UserPromptSubmit" {
		return 0
	}
	if value, ok := payload["transcript_path"]; ok && value != nil && value != "" {
		path, ok := value.(string)
		if !ok {
			return failure()
		}
		f, err := os.Open(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return failure()
		}
		if err == nil {
			line, _ := bufio.NewReader(io.LimitReader(f, 131072)).ReadBytes('\n')
			f.Close()
			var first struct {
				Payload struct {
					Source json.RawMessage `json:"source"`
				} `json:"payload"`
			}
			if json.Unmarshal(line, &first) != nil {
				return failure()
			}
			var source map[string]any
			if json.Unmarshal(first.Payload.Source, &source) == nil {
				if _, ok := source["subagent"]; ok {
					return 0
				}
			}
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return failure()
	}
	argv := []string{"env", "HERDR_SOCKET_PATH=" + socket, exe, "agents", "task-titles", "set", "--pane", paneID, "--session-id", session, "--title"}
	for i := range argv {
		argv[i] = shellQuote(argv[i])
	}
	_ = json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": "UserPromptSubmit", "additionalContext": hookGuidance + strings.Join(argv, " ") + " 'Короткое название задачи'"}})
	return 0
}
