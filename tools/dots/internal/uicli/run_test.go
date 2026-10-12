package uicli

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"dots/native/internal/testpty"
)

func TestMain(m *testing.M) {
	if os.Getenv("DOTS_UI_CLI") == "1" {
		args := os.Args[1:]
		if len(args) > 0 && args[0] == "ui" {
			args = args[1:]
		}
		os.Exit(Run(args))
	}
	os.Exit(m.Run())
}
func command(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = []string{"HOME=" + t.TempDir(), "DOTS_UI_CLI=1", "PATH=" + t.TempDir(), "TERM=xterm-256color", "NO_COLOR=1", "GORACE=atexit_sleep_ms=0"}
	return cmd
}
func TestNativeSelectorKeepsOriginalDataAndHiddenMarks(t *testing.T) {
	cmd := command(t, "select", "--multi", "--title", "Artifacts")
	cmd.Stdin = strings.NewReader("first\tONE\nsecond\tTWO\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	p := testpty.Start(t, cmd)
	p.Screen("Artifacts")
	p.Send("first ")
	p.Screen("1 selected")
	offset := len(p.Text())
	p.Send("\x0csecond ")
	p.After(offset, "2 selected")
	offset = len(p.Text())
	p.Send("\x0cmissing")
	p.After(offset, "Нет совпадений")
	p.Send("\r")
	p.Wait(0)
	if out.String() != "first\tONE\nsecond\tTWO\n" {
		t.Fatalf("selected data %q", out.String())
	}
}
func TestNativeSecretInputPasteClearResizeAndCancel(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		cmd := command(t, "input", "--secret", "--title", "Credential")
		var out bytes.Buffer
		cmd.Stdout = &out
		p := testpty.Start(t, cmd)
		p.Screen("Credential")
		p.Send("\x1b[200~fixture-secret\x1b[201~")
		p.Screen("•••")
		if strings.Contains(p.Text(), "fixture-secret") {
			t.Fatal("secret displayed")
		}
		p.Resize(40, 14)
		p.Send("\x0c")
		offset := len(p.Text())
		p.Send("\x1b[200~final-token\x1b[201~")
		p.After(offset, "•••")
		if cancel {
			p.Send("\x1b")
			p.Wait(130)
			if out.Len() != 0 {
				t.Fatal("cancellation returned secret")
			}
		} else {
			p.Send("\r")
			p.Wait(0)
			if out.String() != "final-token\n" {
				t.Fatalf("value %q", out.String())
			}
		}
		if strings.Contains(p.Text(), "final-token") {
			t.Fatal("secret displayed in terminal")
		}
	}
}
func TestNativeConfirmNeverApprovesEmptyOrPastedEnter(t *testing.T) {
	for _, input := range []string{"\r", "no\r", "\x1b", "\x03", "\x1b[200~yes\n\x1b[201~"} {
		cmd := command(t, "confirm", "--title", "Delete")
		var out bytes.Buffer
		cmd.Stdout = &out
		p := testpty.Start(t, cmd)
		p.Screen("Введите yes")
		p.Send(input)
		if strings.Contains(input, "200~") {
			p.Screen("yes")
			p.Send("\x1b")
		}
		p.Wait(130)
		if out.Len() != 0 {
			t.Fatal(out.String())
		}
	}
	cmd := command(t, "confirm", "--title", "Delete")
	p := testpty.Start(t, cmd)
	p.Screen("Введите yes")
	p.Send("yes\r")
	p.Wait(0)
}
func TestSnapshotAndMessageAreRawInPipe(t *testing.T) {
	for _, action := range []string{"snapshot", "message"} {
		cmd := command(t, action)
		raw := "raw\tdata\nwithout final newline"
		cmd.Stdin = strings.NewReader(raw)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		out, err := cmd.Output()
		if err != nil || string(out) != raw {
			t.Fatalf("%s: %v %q", action, err, out)
		}
	}
}
func TestSelectorWithoutTTYDoesNotReadAnswerFromStdin(t *testing.T) {
	cmd := command(t, "select")
	cmd.Stdin = strings.NewReader("one\ntwo\n")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.Output()
	if err == nil || len(out) != 0 {
		t.Fatalf("selection without tty %v %q", err, out)
	}
}
