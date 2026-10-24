package agents

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"dots/native/internal/testpty"
)

func TestMRInputSIGTERMRestoresTerminal(t *testing.T) {
	f := newAIFixture(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "--mode", "mr")
	cmd.Env = append(os.Environ(), "AGENTS_AI_CLI=1", "TERM=xterm-256color", "NO_COLOR=1")
	p := testpty.Start(t, cmd)
	p.Screen("Целевая ветка MR")
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	p.Wait(1)
	if len(f.logs()) != 0 {
		t.Fatal("termination executed commands")
	}
}

func TestNativeMRInputCancellationAndBranchArgv(t *testing.T) {
	for _, cancel := range []bool{true, false} {
		f := newAIFixture(t)
		f.write("diff", "fixture diff")
		server := localHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"choices":[{"message":{"content":"Fixture MR"}}]}`))
		})
		f.config(server.URL)
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(exe, "--mode", "mr")
		cmd.Env = append(os.Environ(), "AGENTS_AI_CLI=1", "TERM=xterm-256color", "NO_COLOR=1")
		p := testpty.Start(t, cmd)
		p.Screen("Целевая ветка MR")
		if cancel {
			p.Send("\x1b")
			p.Wait(130)
			if len(f.logs()) != 0 {
				t.Fatal("cancel executed git/API")
			}
		} else {
			p.Send("\x1b[200~main\x1b[201~")
			p.Screen("main")
			p.Send("\r")
			p.Wait(0)
			found := false
			for _, call := range f.logs() {
				if call.Name == "git" && strings.Join(call.Args, "|") == "diff|main..feature-fixture" {
					found = true
				}
			}
			if !found {
				t.Fatalf("branch argv %+v", f.logs())
			}
		}
	}
}
