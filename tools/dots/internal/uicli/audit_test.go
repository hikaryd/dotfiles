package uicli

import (
	"bytes"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"dots/native/internal/testpty"
)

func TestSelectorDisplaysSafeLabelsReturnsOriginal(t *testing.T) {
	cmd := command(t, "select", "--title", "Safe selector")
	raw := "label\x1b]52;c;WA==\a"
	cmd.Stdin = strings.NewReader(raw + "\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	p := testpty.Start(t, cmd)
	p.Screen("Safe selector")
	if strings.Contains(p.Text(), "52;c;WA==") {
		t.Fatal("terminal command displayed")
	}
	p.Send("\r")
	p.Wait(0)
	if out.String() != raw+"\n" {
		t.Fatal("selection data changed")
	}
}

func TestPipeInputCancellation(t *testing.T) {
	for _, action := range []string{"message", "snapshot", "select"} {
		t.Run(action, func(t *testing.T) {
			cmd := command(t, action)
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			cmd.Stdin = reader
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Process.Kill()
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			ticker := time.NewTicker(25 * time.Millisecond)
			defer ticker.Stop()
			deadline := time.NewTimer(2 * time.Second)
			defer deadline.Stop()
			time.Sleep(150 * time.Millisecond)
			for {
				select {
				case <-ticker.C:
					cmd.Process.Signal(syscall.SIGTERM)
				case err = <-done:
					if cmd.ProcessState.ExitCode() != 130 {
						t.Fatalf("exit=%d err=%v output=%q", cmd.ProcessState.ExitCode(), err, out.String())
					}
					return
				case <-deadline.C:
					t.Fatal("blocked stdin ignored cancellation")
				}
			}
		})
	}
}
