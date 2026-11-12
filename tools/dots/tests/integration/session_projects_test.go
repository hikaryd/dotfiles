package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSessionsProjectDiscoveryKeepsInventoryAndReusesLivePath(t *testing.T) {
	f := newFixture(t)
	project := filepath.Join(f.home, "Documents/dev", "Sample")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	rows, _ := json.Marshal([]map[string]string{{"Src": "zoxide", "Name": "Sample", "Path": project}})
	f.fake("sesh", allRule(string(rows), 0))
	f.fake("tmux", fakeRule{Args: []string{"list-sessions"}, Prefix: true, Output: "$1\t1\tRenamed session\n"},
		fakeRule{Args: []string{"list-panes"}, Prefix: true, Output: "$1\t" + project + "\n"},
		fakeRule{Args: []string{"attach-session", "-t", "$1"}})
	out, diagnostic, status := f.run("", "sessions", "list", "--json")
	var entries []struct {
		Source string `json:"source"`
		Name   string `json:"name"`
	}
	if status != 0 || json.Unmarshal([]byte(out), &entries) != nil || len(entries) != 2 {
		t.Fatalf("inventory: %d %s %s", status, out, diagnostic)
	}
	_, diagnostic, status = f.run("", "sessions", "open", "Sample")
	if status != 0 {
		t.Fatalf("existing project: %d %s", status, diagnostic)
	}
	calls := f.calls()
	last := calls[len(calls)-1]
	if !reflect.DeepEqual(last.Args, []string{"attach-session", "-t", "$1"}) {
		t.Fatalf("project duplicated live session: %+v", calls)
	}
}

func TestNativeSessionsCreateProjectByNameWithoutHistory(t *testing.T) {
	for _, direct := range []bool{false, true} {
		name := "picker"
		if direct {
			name = "explicit-name"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			project := filepath.Join(f.home, "Documents/dev", "New project 界")
			if err := os.MkdirAll(project, 0700); err != nil {
				t.Fatal(err)
			}
			f.fake("sesh", allRule("[]", 0)) // Project was never visited/zoxide-ranked.
			f.fake("tmux",
				fakeRule{Args: []string{"list-sessions"}, Prefix: true, Output: "$1\t1\tdots\n"},
				fakeRule{Args: []string{"list-panes"}, Prefix: true},
				fakeRule{Args: []string{"new-session"}, Prefix: true, Output: "$2\n"},
				fakeRule{Args: []string{"attach-session", "-t", "$2"}},
			)
			if direct {
				_, diagnostic, status := f.run("", "sessions", "open", filepath.Base(project))
				if status != 0 {
					t.Fatalf("project name: %d %s", status, diagnostic)
				}
			} else {
				p := f.terminal("sessions", "open")
				p.Screen("1/1 rows")
				if strings.Contains(p.Text(), filepath.Base(project)) {
					t.Fatal("unused project clutters empty search")
				}
				p.Send("New project")
				p.Screen("[project · new shell]")
				offset := len(p.Text())
				p.Send("\x0c")
				p.After(offset, "1/1 rows")
				if strings.Contains(p.Text()[offset:], "[project · new shell]") {
					t.Fatal("project stays visible after search clear")
				}
				offset = len(p.Text())
				p.Send("New project")
				p.After(offset, "[project · new shell]")
				p.Send("\r")
				p.Wait(0)
			}
			calls := f.calls()
			created := 0
			for _, c := range calls {
				if c.Name == "tmux" && c.Args[0] == "new-session" {
					created++
					if !strings.Contains(strings.Join(c.Args, "\x00"), "-c\x00"+project) {
						t.Fatalf("session cwd: %q", c.Args)
					}
				}
			}
			last := calls[len(calls)-1]
			if created != 1 || !reflect.DeepEqual(last.Args, []string{"attach-session", "-t", "$2"}) {
				t.Fatalf("project dispatch: %+v", calls)
			}
		})
	}
}
