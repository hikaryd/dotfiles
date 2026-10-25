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
	snapshot, _ := json.Marshal(map[string]any{"result": map[string]any{"snapshot": map[string]any{
		"workspaces": []any{map[string]any{"workspace_id": "w1", "label": "Renamed workspace", "tab_count": 1, "pane_count": 1}},
		"panes":      []any{map[string]any{"workspace_id": "w1", "cwd": project}},
	}}})
	rows, _ := json.Marshal([]map[string]string{{"Src": "zoxide", "Name": "Sample", "Path": project}})
	f.fake("sesh", allRule(string(rows), 0))
	f.fake("herdr", fakeRule{Args: []string{"api", "snapshot"}, Output: string(snapshot)}, fakeRule{Args: []string{"workspace", "focus", "w1"}, Output: `{"result":{}}`})
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
	if !reflect.DeepEqual(last.Args, []string{"workspace", "focus", "w1"}) {
		t.Fatalf("project duplicated live workspace: %+v", calls)
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
			f.fake("herdr",
				fakeRule{Args: []string{"api", "snapshot"}, Output: `{"result":{"snapshot":{"workspaces":[{"workspace_id":"w1","label":"dots","tab_count":1,"pane_count":1}],"panes":[]}}}`},
				fakeRule{Args: []string{"workspace", "create", "--cwd", project, "--label", filepath.Base(project), "--no-focus"}, Output: `{"result":{"workspace":{"workspace_id":"w2"},"tab":{"tab_id":"w2:t1"},"root_pane":{"pane_id":"w2:p1"}}}`},
				fakeRule{Args: []string{"workspace", "report-metadata"}, Prefix: true, Output: `{"result":{}}`},
				fakeRule{Args: []string{"workspace", "focus", "w2"}, Output: `{"result":{}}`},
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
				if c.Name == "tmux" {
					t.Fatal("project creation touched tmux")
				}
				if c.Name == "herdr" && len(c.Args) > 1 && c.Args[0] == "workspace" && c.Args[1] == "create" {
					created++
				}
			}
			last := calls[len(calls)-1]
			if created != 1 || !reflect.DeepEqual(last.Args, []string{"workspace", "focus", "w2"}) {
				t.Fatalf("project dispatch: %+v", calls)
			}
		})
	}
}
