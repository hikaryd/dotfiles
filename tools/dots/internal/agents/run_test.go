package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
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
