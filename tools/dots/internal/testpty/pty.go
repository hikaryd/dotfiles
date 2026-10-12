// Package testpty drives real CLI terminal contracts without fake UI backends.
package testpty

import (
	"bytes"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type CLI struct {
	t             *testing.T
	master, slave *os.File
	cmd           *exec.Cmd
	before        *term.State
	mu            sync.Mutex
	capture       bytes.Buffer
	done          chan error
}

func Start(t *testing.T, cmd *exec.Cmd) *CLI {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := pty.Setsize(master, &pty.Winsize{Rows: 28, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	before, err := term.GetState(int(master.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	p := &CLI{t: t, master: master, slave: slave, cmd: cmd, before: before, done: make(chan error, 1)}
	if cmd.Stdin == nil {
		cmd.Stdin = slave
	}
	if cmd.Stdout == nil {
		cmd.Stdout = slave
	}
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 2}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		buffer := make([]byte, 8192)
		for {
			n, err := master.Read(buffer)
			if n > 0 {
				p.mu.Lock()
				p.capture.Write(buffer[:n])
				p.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); master.Close(); slave.Close() })
	return p
}
func (p *CLI) Text() string { p.mu.Lock(); defer p.mu.Unlock(); return p.capture.String() }
func (p *CLI) Send(text string) {
	p.t.Helper()
	if _, err := p.master.Write([]byte(text)); err != nil {
		p.t.Fatal(err)
	}
}
func (p *CLI) Screen(text string) { p.After(0, text) }
func (p *CLI) After(offset int, text string) {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data := p.Text()
		if len(data) >= offset && strings.Contains(data[offset:], text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.t.Fatalf("new screen did not show %q: %q", text, p.Text())
}
func (p *CLI) Resize(columns, rows uint16) {
	p.t.Helper()
	if err := pty.Setsize(p.master, &pty.Winsize{Rows: rows, Cols: columns}); err != nil {
		p.t.Fatal(err)
	}
}
func (p *CLI) Wait(code int) {
	p.t.Helper()
	select {
	case err := <-p.done:
		got := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				got = e.ExitCode()
			} else {
				p.t.Fatal(err)
			}
		}
		if got != code {
			p.t.Fatalf("exit=%d want=%d screen=%q", got, code, p.Text())
		}
		state, err := term.GetState(int(p.master.Fd()))
		if err != nil || !reflect.DeepEqual(p.before, state) {
			p.t.Fatalf("terminal not restored: %v", err)
		}
	case <-time.After(15 * time.Second):
		p.t.Fatalf("CLI did not exit: %q", p.Text())
	}
}
