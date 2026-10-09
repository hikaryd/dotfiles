package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestTitleSanitization(t *testing.T) {
	for _, tt := range []struct {
		in    string
		limit int
		want  string
	}{
		{"\x1b[31mПочинить\x1b[0m   поиск\u202e | podbor", 32, "Починить поиск"},
		{"hello long world | repo", 13, "hello long…"},
		{"Очень длинное название задачи которое нужно обрезать | repo", 32, "Очень длинное название задачи…"},
	} {
		if got := taskTitle(tt.in, tt.limit, "codex"); got != tt.want {
			t.Errorf("got %q want %q", got, tt.want)
		}
	}
}

type titleFixture struct {
	mu       sync.Mutex
	t        *testing.T
	snapshot snapshot
	calls    []titleCall
	fail     string
	dir      string
}
type titleCall struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func titlesFixture(t *testing.T) *titleFixture {
	t.Helper()
	f := &titleFixture{t: t, dir: t.TempDir(), snapshot: snapshot{
		Panes: []*pane{{ID: "w1:p1", Terminal: "terminal-one", Tab: "w1:t1", Agent: "codex", Native: "Исправить поиск статуса | podbor"}},
		Tabs:  []*tab{{ID: "w1:t1", Label: "nu"}}, Layouts: []layout{{Tab: "w1:t1", Focus: "w1:p1"}},
	}}
	// A short fixture-only socket path also works on Darwin's 104-byte sockaddr.
	socketDir, err := os.MkdirTemp("", "at-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("sandbox blocks real Unix socket fixture: %v", err)
		}
		t.Fatalf("temporary Unix socket unavailable: %v", err)
	}
	t.Setenv("HOME", f.dir)
	t.Setenv("HERDR_SOCKET_PATH", socket)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(f.dir, "state"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				var req titleCall
				if json.NewDecoder(conn).Decode(&req) != nil {
					return
				}
				if req.Method == "session.snapshot" {
					_ = json.NewEncoder(conn).Encode(map[string]any{"result": map[string]any{"snapshot": f.snapshot}})
					return
				}
				f.calls = append(f.calls, req)
				if req.Method == f.fail {
					_ = json.NewEncoder(conn).Encode(map[string]any{"error": map[string]string{"code": "unavailable", "message": "private task content"}})
					return
				}
				switch req.Method {
				case "tab.rename":
					for _, tab := range f.snapshot.Tabs {
						if tab.ID == req.Params["tab_id"] {
							tab.Label = req.Params["label"].(string)
						}
					}
				case "pane.report_metadata":
					for _, pane := range f.snapshot.Panes {
						if pane.ID == req.Params["pane_id"] {
							if req.Params["clear_title"] == true {
								pane.Title = ""
							} else {
								pane.Title = req.Params["title"].(string)
							}
						}
					}
				}
				_ = json.NewEncoder(conn).Encode(map[string]any{"result": map[string]any{}})
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	return f
}
func (f *titleFixture) run(args ...string) (int, string) {
	f.t.Helper()
	f.mu.Lock()
	f.mu.Unlock()
	var out, errout bytes.Buffer
	code := run(append([]string{"task-titles"}, args...), strings.NewReader(""), &out, &errout)
	f.mu.Lock()
	f.mu.Unlock()
	if out.Len() != 0 {
		f.t.Errorf("unexpected stdout %q", out.String())
	}
	return code, errout.String()
}
func (f *titleFixture) sync() {
	f.t.Helper()
	if code, e := f.run("sync"); code != 0 {
		f.t.Fatalf("sync %d: %s", code, e)
	}
}
func (f *titleFixture) set(title, session, agent string) {
	f.t.Helper()
	args := []string{"set", "--pane", "w1:p1", "--title", title, "--agent", agent}
	if session != "" {
		args = append(args, "--session-id", session)
	}
	if code, e := f.run(args...); code != 0 {
		f.t.Fatalf("set %d: %s", code, e)
	}
}
func (f *titleFixture) state() *titleState {
	f.t.Helper()
	path, err := stateFile()
	if err != nil {
		f.t.Fatal(err)
	}
	st, err := readState(path)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}
func (f *titleFixture) want(title, label string) {
	f.t.Helper()
	if f.snapshot.Panes[0].Title != title || f.snapshot.Tabs[0].Label != label {
		f.t.Fatalf("got title %q label %q; want %q %q", f.snapshot.Panes[0].Title, f.snapshot.Tabs[0].Label, title, label)
	}
}
func (f *titleFixture) quiet() {
	f.t.Helper()
	f.calls = nil
	f.sync()
	if len(f.calls) != 0 {
		f.t.Fatalf("feedback loop: %#v", f.calls)
	}
}
func TestTitleSocketOwnership(t *testing.T) {
	t.Run("native title and no feedback", func(t *testing.T) {
		f := titlesFixture(t)
		f.sync()
		f.want("Исправить поиск статуса", "Исправить поиск статуса")
		if f.calls[0].Params["source"] != titleSource || f.calls[0].Params["agent"] != "codex" {
			t.Fatal(f.calls)
		}
		f.quiet()
	})
	t.Run("focus count and previous selected pane", func(t *testing.T) {
		f := titlesFixture(t)
		f.snapshot.Panes = append(f.snapshot.Panes, &pane{ID: "w1:p2", Tab: "w1:t1", Agent: "codex", Native: "Проверить API | podbor"})
		f.sync()
		f.want("Исправить поиск статуса", "Исправить поиск статуса · 2")
		f.snapshot.Layouts[0].Focus = "w1:p2"
		f.sync()
		f.want("Исправить поиск статуса", "Проверить API · 2")
		f.snapshot.Layouts[0].Focus = "unknown"
		f.quiet()
	})
	t.Run("manual tab rename including shell name", func(t *testing.T) {
		f := titlesFixture(t)
		f.sync()
		f.snapshot.Tabs[0].Label = "zsh"
		f.sync()
		f.snapshot.Panes[0].Native = "Другая задача | podbor"
		f.sync()
		f.want("Другая задача", "zsh")
		if !f.state().Tabs["w1:t1"].Manual {
			t.Fatal("not manual")
		}
	})
	t.Run("manual pane and foreign metadata", func(t *testing.T) {
		f := titlesFixture(t)
		f.snapshot.Tabs[0].Label = "Важная работа"
		f.snapshot.Panes[0].Label = "Моя панель"
		f.quiet()
		f.snapshot.Panes[0].Label = ""
		f.snapshot.Panes[0].Title = "Other integration"
		f.quiet()
	})
	t.Run("exit cleanup", func(t *testing.T) {
		f := titlesFixture(t)
		f.sync()
		f.calls = nil
		f.snapshot.Panes[0].Agent = ""
		f.sync()
		f.want("", "nu")
		if f.calls[0].Params["clear_title"] != true || f.calls[0].Params["source"] != titleSource {
			t.Fatal(f.calls)
		}
		f.quiet()
	})
	t.Run("manual pane releases title", func(t *testing.T) {
		f := titlesFixture(t)
		f.sync()
		f.snapshot.Panes[0].Label = "Pinned"
		f.sync()
		if f.snapshot.Panes[0].Title != "" || len(f.state().Panes) != 0 {
			t.Fatal("still owned")
		}
	})
	t.Run("failed rename ownership and retry", func(t *testing.T) {
		f := titlesFixture(t)
		f.fail = "tab.rename"
		code, e := f.run("sync")
		if code != 1 || strings.Contains(e, "private") {
			t.Fatal(code, e)
		}
		st := f.state()
		if st.Panes["w1:p1"] != "Исправить поиск статуса" {
			t.Fatal(st)
		}
		if r := st.Tabs["w1:t1"]; r != nil && r.Last != "nu" {
			t.Fatal(r)
		}
		f.fail = ""
		f.sync()
		f.want("Исправить поиск статуса", "Исправить поиск статуса")
	})
	t.Run("semantic overrides OSC and no loop", func(t *testing.T) {
		f := titlesFixture(t)
		f.sync()
		f.set("Статусы заявок · поиск закрытых", "session-one", "codex")
		f.snapshot.Panes[0].Native = "Исследовать баг | podbor"
		f.sync()
		f.want("Статусы заявок · поиск закрытых", "Статусы заявок · поиск закрытых")
		f.quiet()
	})
	t.Run("semantic session and terminal replacement", func(t *testing.T) {
		f := titlesFixture(t)
		f.set("Точная задача", "session-one", "codex")
		p := f.snapshot.Panes[0]
		p.Session = &agentSession{"codex", "session-two"}
		f.sync()
		if len(f.state().Semantic) != 0 {
			t.Fatal("stale session")
		}
		f.want("Исправить поиск статуса", "Исправить поиск статуса")
		f.set("Новая задача", "session-two", "codex")
		p.Terminal = "replacement"
		f.sync()
		if len(f.state().Semantic) != 0 {
			t.Fatal("stale terminal")
		}
	})
	t.Run("late session binding and exit", func(t *testing.T) {
		f := titlesFixture(t)
		f.set("Точная задача", "", "codex")
		p := f.snapshot.Panes[0]
		p.Session = &agentSession{"codex", "session-one"}
		f.sync()
		if *f.state().Semantic[p.ID].Session != "session-one" {
			t.Fatal("not bound")
		}
		p.Agent = ""
		f.sync()
		if len(f.state().Semantic) != 0 || p.Title != "" {
			t.Fatal("not cleared")
		}
	})
	t.Run("reject invalid semantic targets and preserve labels", func(t *testing.T) {
		f := titlesFixture(t)
		p := f.snapshot.Panes[0]
		p.Session = &agentSession{"codex", "session-one"}
		for _, args := range [][]string{{"--pane", "w1:p1", "--title", "Title", "--session-id", "other-session"}, {"--pane", "missing", "--title", "Title"}, {"--pane", "w1:p1", "--title", " \u202e "}} {
			if code, _ := f.run(append([]string{"set"}, args...)...); code != 1 {
				t.Fatal(code)
			}
		}
		p.Label = "Pinned pane"
		f.snapshot.Tabs[0].Label = "Pinned tab"
		f.set("Точная задача", "session-one", "codex")
		if len(f.calls) != 0 {
			t.Fatal(f.calls)
		}
		p.Terminal = ""
		if code, _ := f.run("set", "--pane", p.ID, "--title", "Title"); code != 1 {
			t.Fatal(code)
		}
	})
	t.Run("metadata acknowledgement keeps ownership", func(t *testing.T) {
		f := titlesFixture(t)
		f.sync()
		if f.state().Panes["w1:p1"] != "Исправить поиск статуса" {
			t.Fatal("no ownership")
		}
		f.set("3742879 · Создание вакансии", "", "codex")
		if f.state().Panes["w1:p1"] != "3742879 · Создание вакансии" {
			t.Fatal("no semantic ownership")
		}
	})
	t.Run("dead pane and tab records pruned", func(t *testing.T) {
		f := titlesFixture(t)
		f.set("Title", "s", "codex")
		f.snapshot.Panes = []*pane{}
		f.snapshot.Tabs = []*tab{}
		f.sync()
		st := f.state()
		if len(st.Panes)+len(st.Tabs)+len(st.Semantic) != 0 {
			t.Fatal(st)
		}
	})
}
func TestPiTitleSocket(t *testing.T) {
	t.Run("Pi native title complex directory and no loop", func(t *testing.T) {
		f := titlesFixture(t)
		p := f.snapshot.Panes[0]
		p.Agent = "pi"
		p.Cwd = "/projects/my - repo"
		p.Native = "π - Поиск - API | UI - my - repo"
		f.snapshot.Tabs[0].Label = "pi"
		f.sync()
		f.want("Поиск - API | UI", "Поиск - API | UI")
		if f.calls[0].Params["agent"] != "pi" {
			t.Fatal(f.calls)
		}
		f.quiet()
	})
	t.Run("unnamed and foreground directory", func(t *testing.T) {
		f := titlesFixture(t)
		p := f.snapshot.Panes[0]
		p.Agent = "pi"
		p.Cwd = "/projects/my - repo"
		p.Native = "π - my - repo"
		f.quiet()
		p.Cwd = ""
		p.Native = "π - repo"
		f.quiet()
		p.ForegroundCwd = "/projects/my - repo"
		p.Native = "π - Поиск - my - repo"
		f.sync()
		f.want("Поиск", "Поиск")
	})
	t.Run("semantic Pi and explicit empty masks stale OSC", func(t *testing.T) {
		f := titlesFixture(t)
		p := f.snapshot.Panes[0]
		p.Agent = "pi"
		p.Native = "π - Старое имя - repo"
		f.set("Название сессии | API", "pi-session", "pi")
		f.want("Название сессии | API", "Название сессии | API")
		f.set("", "new-pi-session", "pi")
		f.want("", "nu")
		f.quiet()
		if code, _ := f.run("set", "--agent", "pi", "--pane", p.ID, "--title", " \u202e "); code != 1 {
			t.Fatal("whitespace accepted", code)
		}
	})
	t.Run("agent replacement same terminal", func(t *testing.T) {
		f := titlesFixture(t)
		f.set("Codex задача", "same-id", "codex")
		p := f.snapshot.Panes[0]
		p.Agent = "pi"
		p.Native = "π - Pi задача - repo"
		f.sync()
		if len(f.state().Semantic) != 0 {
			t.Fatal("old agent")
		}
		f.want("Pi задача", "Pi задача")
		f.set("Pi имя", "pi-id", "pi")
		p.Agent = "codex"
		p.Native = "Codex снова | repo"
		f.sync()
		if len(f.state().Semantic) != 0 {
			t.Fatal("old pi")
		}
		f.want("Codex снова", "Codex снова")
	})
	t.Run("Pi identity terminal replacement and exit", func(t *testing.T) {
		f := titlesFixture(t)
		p := f.snapshot.Panes[0]
		p.Agent = "pi"
		p.Native = "π - repo"
		p.Session = &agentSession{"pi", "pi-one"}
		for _, args := range [][]string{{"--agent", "pi", "--session-id", "pi-two"}, {"--session-id", "pi-one"}} {
			if code, _ := f.run(append([]string{"set", "--pane", p.ID, "--title", "Title"}, args...)...); code != 1 {
				t.Fatal(code)
			}
		}
		f.set("Pi имя", "pi-one", "pi")
		p.Session.Value = "pi-two"
		f.sync()
		if len(f.state().Semantic) != 0 || p.Title != "" {
			t.Fatal("session survives")
		}
		f.set("Pi имя", "pi-two", "pi")
		p.Terminal = "new-terminal"
		f.sync()
		if len(f.state().Semantic) != 0 {
			t.Fatal("terminal survives")
		}
		f.set("Pi имя", "pi-two", "pi")
		p.Agent = ""
		f.sync()
		if len(f.state().Semantic) != 0 {
			t.Fatal("agent survives")
		}
		f.want("", "nu")
	})
	t.Run("legacy Codex state", func(t *testing.T) {
		f := titlesFixture(t)
		path, _ := stateFile()
		if err := os.WriteFile(path, []byte(`{"panes":{},"tabs":{},"semantic":{"w1:p1":{"title":"Старое имя","terminal_id":"terminal-one","session_id":"session-one"}}}`), 0600); err != nil {
			t.Fatal(err)
		}
		f.sync()
		f.want("Старое имя", "Старое имя")
	})
	t.Run("mixed agents focus and manual names", func(t *testing.T) {
		f := titlesFixture(t)
		p := &pane{ID: "w1:p2", Terminal: "terminal-two", Tab: "w1:t1", Agent: "pi", Native: "π - Проверить API - repo"}
		f.snapshot.Panes = append(f.snapshot.Panes, p)
		f.sync()
		f.want("Исправить поиск статуса", "Исправить поиск статуса · 2")
		f.snapshot.Layouts[0].Focus = p.ID
		f.sync()
		f.want("Исправить поиск статуса", "Проверить API · 2")
		p.Label = "Моя панель"
		f.snapshot.Tabs[0].Label = "Моя вкладка"
		if code, e := f.run("set", "--pane", p.ID, "--title", "Другая задача", "--session-id", "pi-id", "--agent", "pi"); code != 0 {
			t.Fatal(e)
		}
		if p.Title != "" || f.snapshot.Tabs[0].Label != "Моя вкладка" {
			t.Fatal("manual ownership")
		}
	})
}

// These file-seam checks remain useful when the sandbox denies listening sockets.
// Socket integration above is independently required and never substituted here.
func TestPersistedSemanticIdentity(t *testing.T) {
	for _, agent := range []string{"codex", "pi"} {
		t.Run(agent, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			st := newState()
			p := &pane{ID: "p", Terminal: "term", Agent: agent, Native: "native | repo"}
			s := snapshot{Panes: []*pane{p}}
			if err := setTitle(s, st, "p", "Смысл задачи", "", agent); err != nil {
				t.Fatal(err)
			}
			if err := writeState(path, st); err != nil {
				t.Fatal(err)
			}
			st, err := readState(path)
			if err != nil {
				t.Fatal(err)
			}
			if st.Semantic["p"].Session != nil {
				t.Fatal("premature identity")
			}
			panes := map[string]*pane{"p": p}
			p.Session = &agentSession{Agent: agent, Value: "first"}
			desired := semanticTitles(panes, st)
			if desired["p"] != "Смысл задачи" || st.Semantic["p"].Session == nil || *st.Semantic["p"].Session != "first" {
				t.Fatal("late binding failed")
			}
			if err := writeState(path, st); err != nil {
				t.Fatal(err)
			}
			st, err = readState(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := setTitle(s, st, "p", "mismatch", "second", agent); err == nil {
				t.Fatal("session mismatch accepted")
			}
			p.Session.Value = "second"
			semanticTitles(panes, st)
			if len(st.Semantic) != 0 {
				t.Fatal("stale session survived")
			}
			if agent == "pi" {
				p.Native = "π - Старое имя - repo"
				if err := setTitle(s, st, "p", "", "second", agent); err != nil {
					t.Fatal(err)
				}
				desired = semanticTitles(panes, st)
				if desired["p"] != "" {
					t.Fatal("empty Pi override lost")
				}
			}
			if err := setTitle(s, st, "p", "Задача", "second", agent); err != nil {
				t.Fatal(err)
			}
			p.Terminal = "replacement"
			semanticTitles(panes, st)
			if len(st.Semantic) != 0 {
				t.Fatal("stale terminal survived")
			}
			if err := setTitle(s, st, "p", "Задача", "second", agent); err != nil {
				t.Fatal(err)
			}
			p.Agent = ""
			semanticTitles(panes, st)
			if len(st.Semantic) != 0 {
				t.Fatal("exited agent survived")
			}
		})
	}
}
func TestUnicodeTitleDetails(t *testing.T) {
	cases := map[string]string{
		"\x1b]0;private\x07Название\x1b]0;private\x1b\\": "Название",
		"Title\u0378\ue000\u200d\x00":                    "Title",
		strings.Repeat("😀", 40):                          strings.Repeat("😀", 31) + "…",
	}
	for input, want := range cases {
		if got := taskTitle(input, 32, "pi"); got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
	for _, label := range []string{"123", "١٢٣", "pi", "codex", "nu"} {
		if !automaticLabel(label) {
			t.Errorf("not automatic: %q", label)
		}
	}
	for _, label := range []string{"", "Manual", "½", "²"} {
		if automaticLabel(label) {
			t.Errorf("incorrect automatic: %q", label)
		}
	}
}
