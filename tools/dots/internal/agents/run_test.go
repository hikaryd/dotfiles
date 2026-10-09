package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStateFilesAndLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(home, "state"))
	t.Setenv("HERDR_SOCKET_PATH", "fixture-socket-one")
	path, err := stateFile()
	if err != nil {
		t.Fatal(err)
	}
	first := path
	st, err := readState(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Semantic["pane"] = &semanticRecord{Title: "Имя", Terminal: "term"}
	if err = writeState(path, st); err != nil {
		t.Fatal(err)
	}
	got, err := readState(path)
	if err != nil || got.Semantic["pane"].Title != "Имя" {
		t.Fatal(got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	t.Setenv("HERDR_SOCKET_PATH", "fixture-socket-two")
	path, err = stateFile()
	if err != nil || path == first {
		t.Fatal(path, err)
	}
	unlock, err := lockState(path, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = lockState(path, 60*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "busy") || time.Since(start) > time.Second {
		t.Fatal(err)
	}
	unlock()
	unlock, err = lockState(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	for _, bad := range []string{`null`, `[]`, `{}`, `{"panes":null,"tabs":{}}`, `{"panes":{},"tabs":{},"semantic":[]}`, `{"panes":{},"tabs":{"p":null}}`, `{"panes":{},"tabs":{},"semantic":{"p":null}}`} {
		if _, err := decodeState([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	legacy, err := decodeState([]byte(`{"panes":{},"tabs":{}}`))
	if err != nil || legacy.Semantic == nil {
		t.Fatal(legacy, err)
	}
	t.Setenv("HERDR_SOCKET_PATH", "")
	if _, err := stateFile(); err == nil {
		t.Fatal("socket required")
	}
}
func TestHookStdinAndQuoting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERDR_PANE_ID", "pane 'quoted'; $(nope)")
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(home, "socket 'quoted'"))
	transcript := filepath.Join(home, "transcript.jsonl")
	payload := map[string]any{"session_id": "session 'quoted'; $(nope)", "hook_event_name": "UserPromptSubmit", "transcript_path": transcript}
	invoke := func() (string, int) {
		t.Helper()
		b, _ := json.Marshal(payload)
		var out, errout bytes.Buffer
		code := run([]string{"codex-task-title", "hook"}, bytes.NewReader(b), &out, &errout)
		if errout.Len() != 0 {
			t.Fatal(errout.String())
		}
		return out.String(), code
	}
	output, code := invoke()
	if code != 0 {
		t.Fatal(code)
	}
	var result struct {
		Output struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Output.Event != "UserPromptSubmit" || !strings.Contains(result.Output.Context, "agents task-titles set") || strings.Contains(result.Output.Context, "python") {
		t.Fatal(output)
	}
	command := strings.Split(result.Output.Context, "\n")[1]
	// Parse through a real shell with inert positional parameters, never execute the hook command.
	shell := exec.Command("/bin/sh", "-c", "set -- "+command+"; printf '%s\\n' \"$@\"")
	parsed, err := shell.Output()
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	want := strings.Join([]string{"env", "HERDR_SOCKET_PATH=" + os.Getenv("HERDR_SOCKET_PATH"), exe, "agents", "task-titles", "set", "--pane", os.Getenv("HERDR_PANE_ID"), "--session-id", payload["session_id"].(string), "--title", "Короткое название задачи", ""}, "\n")
	if string(parsed) != want {
		t.Fatalf("argv mismatch %q", parsed)
	}
	if err := os.WriteFile(transcript, []byte("{\"payload\":{\"source\":{\"subagent\":{}}}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, _ := invoke(); got != "" {
		t.Fatal("subagent received context", got)
	}
	if err := os.WriteFile(transcript, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, _ := invoke(); !strings.Contains(got, "systemMessage") {
		t.Fatal(got)
	}
	payload["transcript_path"] = filepath.Join(home, "missing")
	payload["hook_event_name"] = "Other"
	if got, _ := invoke(); got != "" {
		t.Fatal(got)
	}
	payload["hook_event_name"] = "UserPromptSubmit"
	payload["session_id"] = 2
	if got, _ := invoke(); got != "" {
		t.Fatal(got)
	}
	var out bytes.Buffer
	if code := runHook([]string{"hook"}, strings.NewReader("not JSON"), &out); code != 0 || !strings.Contains(out.String(), "systemMessage") {
		t.Fatal(code, out.String())
	}
}
func TestSetJSONStdin(t *testing.T) {
	f := titlesFixture(t)
	p := f.snapshot.Panes[0]
	p.Agent = "pi"
	p.Native = "π - Старое имя - repo"
	f.mu.Lock()
	f.mu.Unlock()
	var out, errout bytes.Buffer
	code := run([]string{"task-titles", "set", "--stdin"}, strings.NewReader(`{"pane":"w1:p1","agent":"pi","title":"Имя | API","session_id":"pi-one"}`), &out, &errout)
	f.mu.Lock()
	f.mu.Unlock()
	if code != 0 {
		t.Fatal(code, errout.String())
	}
	f.want("Имя | API", "Имя | API")
	code = run([]string{"task-titles", "set"}, strings.NewReader(`{"pane_id":"w1:p1","agent":"pi","title":"","session_id":"pi-two"}`), &out, &errout)
	f.mu.Lock()
	f.mu.Unlock()
	if code != 0 {
		t.Fatal(code, errout.String())
	}
	f.want("", "nu")
	f.quiet()
}
func TestHerdrInvalidAcknowledgements(t *testing.T) {
	for _, response := range []string{"", `{}`, `{"error":{"code":"private secret","message":"private secret"}}`, "not json"} {
		t.Run(response, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "ar-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			socket := filepath.Join(dir, "s")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
					t.Skipf("sandbox blocks real Unix socket fixture: %v", err)
				}
				t.Fatal(err)
			}
			defer listener.Close()
			t.Setenv("HERDR_SOCKET_PATH", socket)
			done := make(chan struct{})
			go func() {
				defer close(done)
				c, e := listener.Accept()
				if e != nil {
					return
				}
				defer c.Close()
				var req map[string]any
				_ = json.NewDecoder(c).Decode(&req)
				_, _ = c.Write([]byte(response))
			}()
			_, err = herdrRequest("pane.report_metadata", map[string]any{"title": "private secret"})
			<-done
			if err == nil || strings.Contains(err.Error(), "private secret") {
				t.Fatal(err)
			}
		})
	}
}
func TestCommandValidation(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"task-titles"}, {"task-titles", "set", "--pane", "p"}, {"task-titles", "set", "--pane", "p", "--title", "x", "--agent", "unknown"}, {"ai"}, {"ai", "--mode", "wrong"}, {"ai", "--mode", "commit", "--provider", "wrong"}} {
		var out, errout bytes.Buffer
		if code := run(args, strings.NewReader(""), &out, &errout); code != 2 {
			t.Errorf("args %v code %d", args, code)
		}
	}
}
