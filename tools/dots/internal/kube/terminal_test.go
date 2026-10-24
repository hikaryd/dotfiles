package kube

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"dots/native/internal/testpty"
)

func (f *fixture) terminal(args ...string) *testpty.CLI {
	f.t.Helper()
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(f.env, "TERM=xterm-256color")
	return testpty.Start(f.t, cmd)
}
func TestNativeProdConfirmationRequiresYesAndRestoresTTY(t *testing.T) {
	for _, answer := range []string{"\x1b", "\r", "no\r", "yes\r"} {
		f := newFixture(t)
		f.fake("kubectl", exact(pinned("prod", "delete", "pod/api"), "fixture mutation\n", 0))
		p := f.terminal("kpprod", "delete", "pod/api")
		p.Screen("Введите yes")
		p.Send(answer)
		code := 130
		if answer == "yes\r" {
			code = 0
		}
		p.Wait(code)
		if (len(f.calls()) == 1) != (code == 0) {
			t.Fatalf("unapproved mutation %+v", f.calls())
		}
	}
}
func TestNativeActionPickerCancellationDoesNotMutate(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact(pinned("dev", "get", "cronjobs", "-o", "json"), `{"items":[{"metadata":{"name":"nightly"}}]}`, 0))
	p := f.terminal("kpdev-cron-run")
	p.Screen("run a CronJob now")
	p.Send("\x1b")
	p.Wait(130)
	if len(f.calls()) != 1 {
		t.Fatalf("cancel mutated %+v", f.calls())
	}
}
func TestHeadlessSelectionRejectsAmbiguousExplicitQuery(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact(pinned("dev", "get", "pods", "-o", "json"), `{"items":[{"metadata":{"name":"api-1"}},{"metadata":{"name":"api-2"}}]}`, 0))
	out, diagnostic, code := f.run("", "kpdev-logs", "api")
	if code == 0 || out != "" || len(f.calls()) != 1 || !strings.Contains(diagnostic, "without a terminal") {
		t.Fatalf("ambiguous target %d %q %q", code, out, diagnostic)
	}
}
