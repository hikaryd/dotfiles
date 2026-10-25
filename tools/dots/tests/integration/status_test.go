package integration_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusReusesPaneSnapshot(t *testing.T) {
	for _, backend := range []string{"tmux", "rmux"} {
		t.Run(backend, func(t *testing.T) {
			f := newFixture(t)
			project := f.repo("project with spaces")
			f.registry(project)
			prefix := []string{}
			if backend == "rmux" {
				f.env = append(f.env, "RMUX=/tmp/status-fixture.sock,42,0")
				prefix = []string{"-S", "/tmp/status-fixture.sock"}
			}
			args := func(format string) []string {
				return append(append([]string{}, prefix...), "list-panes", "-a", "-F", format)
			}
			f.fake(backend,
				fakeRule{Args: args("#{session_id}\t#{session_name}\t#{window_id}\t#{pane_id}\t#{session_attached}\t#{pane_current_command}\t#{pane_active}\t#{pane_pid}\t#{pane_current_path}"), Output: "$1\tproject\t@1\t%1\t1\tzsh\t1\t10\t" + project + "\n"},
			)
			f.fake("ps", allRule("10 1 /bin/zsh\n20 10 /bin/server\n", 0))
			f.fake("lsof", fakeRule{Args: []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn0"}, Output: "p20\x00cserver\x00n127.0.0.1:1234\x00\n"}, fakeRule{Args: []string{"-a", "-p", "20", "-d", "cwd", "-Fpn0"}})
			f.fake("git", fakeRule{Args: []string{"-C", project, "config", "--null", "--get-regexp", `^filter\..*\.(clean|process)$`}, Code: 1}, fakeRule{Args: []string{"--no-optional-locks", "-C", project, "-c", "core.fsmonitor=false", "status", "--porcelain=v1", "-z", "--branch", "--untracked-files=normal", "--ignore-submodules=dirty"}, Output: "## main\x00"})
			out, diagnostic, code := f.run("", "status", "--json")
			var report struct {
				Projects []struct {
					Ports []struct {
						PID int
						Via string
					}
				}
			}
			if code != 0 || json.Unmarshal([]byte(out), &report) != nil || len(report.Projects) != 1 || len(report.Projects[0].Ports) != 1 || report.Projects[0].Ports[0].PID != 20 || report.Projects[0].Ports[0].Via != "pane ancestry" {
				t.Fatalf("status attribution: exit %d, %s %s", code, out, diagnostic)
			}
			queries := 0
			for _, call := range f.calls() {
				if call.Name == backend && strings.Contains(strings.Join(call.Args, " "), "list-panes") {
					queries++
				}
			}
			if queries != 1 {
				t.Fatalf("status collected %d pane snapshots, want one consistent snapshot", queries)
			}
		})
	}
}

func TestStatusWithoutListenersSkipsProcessInspection(t *testing.T) {
	f := newFixture(t)
	f.fake("tmux", allRule("", 0))
	f.fake("ps", allRule("", 0))
	f.fake("lsof", fakeRule{Args: []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn0"}, Code: 1})
	out, diagnostic, code := f.run("", "status", "--json")
	if code != 0 || !strings.Contains(out, `"projects": []`) {
		t.Fatalf("empty status: exit %d, %s %s", code, out, diagnostic)
	}
	calls := f.calls()
	if len(calls) != 2 || calls[0].Name != "tmux" || calls[1].Name != "lsof" {
		t.Fatalf("empty listener inventory should need only panes and lsof, got %+v", calls)
	}
}

func TestStatusInvalidPanePIDDoesNotAttributePorts(t *testing.T) {
	f := newFixture(t)
	project := f.repo("project")
	f.registry(project)
	f.fake("tmux", fakeRule{Args: []string{"list-panes"}, Prefix: true, Output: "$1\tproject\t@1\t%1\t1\tzsh\t1\t999999999999999999999999\t" + project + "\n"})
	f.fake("ps", allRule("20 9223372036854775807 /bin/server\n", 0))
	f.fake("lsof", fakeRule{Args: []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn0"}, Output: "p20\x00cserver\x00n127.0.0.1:1234\x00\n"}, fakeRule{Args: []string{"-a", "-p", "20", "-d", "cwd", "-Fpn0"}})
	f.fake("git", allRule("", 1))
	out, diagnostic, code := f.run("", "status", "--json")
	var report struct {
		Projects   []struct{ Ports []any }
		Unassigned []struct{ PID int } `json:"unassigned_ports"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &report) != nil || len(report.Projects) != 1 || len(report.Projects[0].Ports) != 0 || len(report.Unassigned) != 1 || report.Unassigned[0].PID != 20 {
		t.Fatalf("invalid pane PID attributed a port: exit %d, %s %s", code, out, diagnostic)
	}
}
