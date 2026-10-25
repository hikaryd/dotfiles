package integration_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"dots/native/tests/testutil/testpty"

	"golang.org/x/sys/unix"
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
			fd := int(writer.Fd())
			if err := unix.SetNonblock(fd, true); err != nil {
				t.Fatal(err)
			}
			// A full pipe becomes writable only after the CLI starts reading it.
			buffer := make([]byte, 4096)
			for {
				_, err := unix.Write(fd, buffer)
				if errors.Is(err, unix.EAGAIN) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
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
			poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}
			for {
				n, err := unix.Poll(poll, 10000)
				if errors.Is(err, unix.EINTR) {
					continue
				}
				if err != nil || n != 1 || poll[0].Revents&unix.POLLOUT == 0 {
					t.Fatalf("CLI did not start reading stdin: poll=%v err=%v", poll, err)
				}
				break
			}
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if cmd.ProcessState.ExitCode() != 130 {
					t.Fatalf("exit=%d err=%v output=%q", cmd.ProcessState.ExitCode(), err, out.String())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("blocked stdin ignored cancellation")
			}
		})
	}
}
