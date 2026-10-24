package desktop

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseGitPorcelain(t *testing.T) {
	g, e := parseGit("## main...origin/main [ahead 1]\x00M  staged\x00 M dirty\x00?? untracked\x00UU conflict\x00R  renamed\x00old name\x00")
	if e != nil || g.State != "dirty" || g.Branch != "main" || *g.Staged != 2 || *g.Unstaged != 1 || *g.Untracked != 1 || *g.Conflicts != 1 {
		t.Fatalf("git %+v %v", g, e)
	}
	g, e = parseGit("## No commits yet on feature\x00")
	if e != nil || g.State != "clean" || g.Branch != "feature" {
		t.Fatalf("initial %+v %v", g, e)
	}
	for _, s := range []string{"", "?? no header\x00", "## main\x00bad record", "## main\x00R  renamed"} {
		if _, e := parseGit(s); e == nil {
			t.Errorf("invalid porcelain accepted %q", s)
		}
	}
}
func TestStatusLocalAttributionAndGitFilterGuard(t *testing.T) {
	f := newFixture(t)
	p := f.repo("project")
	f.registry(p)
	f.fake("tmux", fakeRule{Args: []string{"list-panes", "-a", "-F", "#{pane_id}\t#{pane_pid}"}, Output: "%1\t10\n"}, fakeRule{Args: []string{"list-panes"}, Prefix: true, Output: "$1\tproject\t@1\t%1\t1\tnvim\t1\t" + p + "\n"})
	f.fake("ps", allRule("10 1 /bin/zsh\n20 10 /bin/server\n30 10 /bin/other\n", 0))
	f.fake("lsof", fakeRule{Args: []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn0"}, Output: "p20\x00cserver\x00n127.0.0.1:1234\x00n127.0.0.1:1234\x00\np30\x00cother\x00n*:9999\x00\n"}, fakeRule{Args: []string{"-a", "-p", "20,30", "-d", "cwd", "-Fpn0"}, Output: "p30\x00n" + f.home + "\x00\n"})
	f.fake("git", fakeRule{Args: []string{"-C", p, "config", "--null", "--get-regexp", `^filter\..*\.(clean|process)$`}, Output: "filter.fixture.clean\nfixture-secret-command\x00"}, fakeRule{Args: []string{"-C", p, "symbolic-ref", "--short", "-q", "HEAD"}, Output: "feature\n"})
	out, err, code := f.run("", "status", "--json")
	var report statusReport
	if code != 0 || json.Unmarshal([]byte(out), &report) != nil || len(report.Projects) != 1 {
		t.Fatalf("report %d %s %s", code, out, err)
	}
	row := report.Projects[0]
	if row.Git.State != "unknown" || row.Git.Branch != "feature" || len(row.Ports) != 1 || row.Ports[0].Via != "pane ancestry" || len(report.Unassigned) != 1 || report.Unassigned[0].PID != 30 {
		t.Fatalf("report %+v", report)
	}
	for _, c := range f.calls() {
		if c.Name == "git" {
			if has("status", c.Args...) {
				t.Fatal("executable filters were not skipped")
			}
			if c.Env["GIT_OPTIONAL_LOCKS"] != "0" {
				t.Fatal("missing read-only environment")
			}
		}
	}
	if strings.Contains(out+err, "fixture-secret-command") {
		t.Fatal("filter command leaked")
	}
}
func TestLsofPathsAndAncestryCycle(t *testing.T) {
	r := parseLsof("p1\x00cfoo\x00n/path with\nnewline\x00\np2\x00cbar\x00n*:80\x00")
	if len(r) != 2 || !reflect.DeepEqual(r[1].names, []string{"/path with\nnewline"}) {
		t.Fatalf("lsof %+v", r)
	}
	if ancestorProject(1, map[int]process{1: {parent: 2}, 2: {parent: 1}}, map[int]string{}) != "" {
		t.Fatal("cycle attributed")
	}
	if ancestorProject(3, map[int]process{3: {parent: 2}}, map[int]string{2: "project"}) != "project" {
		t.Fatal("ancestry missing")
	}
}
func TestStatusUnavailableInventoryFails(t *testing.T) {
	f := newFixture(t)
	out, err, code := f.run("", "status", "--json")
	if code != 1 || out != "" || !strings.Contains(err, "сбор снимка не удался") {
		t.Fatalf("%d %q %q", code, out, err)
	}
}
