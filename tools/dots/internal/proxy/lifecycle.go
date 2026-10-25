package proxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (c *controller) readPID() (int, error) {
	s, err := readText(c.path("xray.pid"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.New("Could not read xray PID file.")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || pid <= 1 {
		return 0, c.removePID()
	}
	return pid, nil
}
func (c *controller) removePID() error {
	err := os.Remove(c.path("xray.pid"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("Could not remove xray PID file.")
	}
	return nil
}
func pidRunning(pid int) bool {
	if pid <= 1 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// splitCommand accepts the escaped spaces/quotes printed by ps without invoking a shell.
func splitCommand(command string) []string {
	var args []string
	var word strings.Builder
	var quote rune
	escaped := false
	active := false
	for _, r := range command {
		if escaped {
			word.WriteRune(r)
			escaped = false
			active = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			active = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			active = true
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if active {
				args = append(args, word.String())
				word.Reset()
				active = false
			}
			continue
		}
		active = true
		word.WriteRune(r)
	}
	if escaped || quote != 0 {
		return nil
	}
	if active {
		args = append(args, word.String())
	}
	return args
}
func ownsCommand(command, config string) bool {
	args := splitCommand(command)
	if len(args) < 4 {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(filepath.Base(args[0])), "xray") || args[1] != "run" {
		return false
	}
	for i := 2; i < len(args)-1; i++ {
		if args[i] == "-c" || args[i] == "-config" || args[i] == "--config" {
			if args[i+1] == config {
				return true
			}
			// Some ps implementations show spaces in argv without escaping them.
			for j := i + 2; j < len(args); j++ {
				if strings.Join(args[i+1:j+1], " ") == config {
					return true
				}
			}
		}
	}
	return false
}
func (c *controller) owned(pid int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "command=")
	b, err := cmd.Output()
	if err != nil {
		return false
	}
	return ownsCommand(strings.TrimSpace(string(b)), c.path("config.json"))
}

// flock is released by the kernel on process exit; only the controller, never xray, inherits it.
func (c *controller) lock() (func(), error) {
	return c.lockFile("controller.lock", 5*time.Second)
}
func (c *controller) lockFile(name string, timeout time.Duration) (func(), error) {
	f, err := os.OpenFile(c.path(name), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, errors.New("Could not open controller lock.")
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return nil, errors.New("Could not secure controller lock.")
	}
	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, errors.New("Could not acquire controller lock.")
		}
		if c.ctx != nil && c.ctx.Err() != nil {
			f.Close()
			return nil, c.ctx.Err()
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, errors.New("Timed out waiting for dots-proxy controller lock.")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
func (c *controller) start(server object) error {
	unlock, err := c.lock()
	if err != nil {
		return err
	}
	defer unlock()
	pid, err := c.readPID()
	if err != nil {
		return err
	}
	if pid > 0 {
		if pidRunning(pid) {
			if c.owned(pid) {
				fmt.Fprintf(c.out, "shared xray is already running (PID %d).\nStop it first: dots-proxy --stop\n", pid)
				return nil
			}
			fmt.Fprintf(c.errOut, "Warning: PID file points to PID %d, but it is not this pi xray process. Removing stale PID file without stopping that process.\n", pid)
		}
		if err := c.removePID(); err != nil {
			return err
		}
	}
	bin, err := exec.LookPath("xray")
	if err != nil {
		return errors.New("xray not found. Install it with: brew install xray")
	}
	cfg, err := c.config(server)
	if err != nil {
		return err
	}
	if err := writeJSON(c.path("config.json"), cfg); err != nil {
		return errors.New("Could not write xray configuration.")
	}
	fmt.Fprintf(c.out, "\nConfig saved: %s\nServer: %s\nAddress: %s:%v\nTransport: %s, Security: %s\n", c.path("config.json"), value(server, "name", ""), value(server, "address", ""), server["port"], value(server, "type", ""), value(server, "security", ""))
	cmd := exec.Command(bin, "run", "-c", c.path("config.json"))
	// Disconnect from the controlling terminal and discard potentially secret-bearing xray errors.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return errors.New("xray failed to start (process launch failed).")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	cleanup := func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		_ = c.removePID()
	}
	select {
	case <-done:
		return errors.New("\nxray failed to start (process exited; diagnostic output withheld because it may contain credentials).")
	case <-time.After(1500 * time.Millisecond):
	}
	if err := atomicWrite(c.path("xray.pid"), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0600); err != nil {
		cleanup()
		return errors.New("Could not save xray PID; started process was stopped.")
	}
	select {
	case <-done:
		_ = c.removePID()
		return errors.New("xray failed to start (process exited).")
	default:
	}
	fmt.Fprintf(c.out, "\nxray started (PID %d)\nHTTP proxy: %s\n\nTest it:\n  curl -x %s -s -o /dev/null -w 'HTTP Status: %%{http_code}\\n' https://api.openai.com/v1/models\nExpected: HTTP 401 without API key. 401 is OK here: it means proxy reached OpenAI.\n", cmd.Process.Pid, c.proxyURL(), c.proxyURL())
	return nil
}
func (c *controller) stop() error {
	unlock, err := c.lock()
	if err != nil {
		return err
	}
	defer unlock()
	pid, err := c.readPID()
	if err != nil {
		return err
	}
	if pid == 0 {
		fmt.Fprintln(c.out, "shared xray is not running (no PID file).")
		return nil
	}
	if !pidRunning(pid) {
		fmt.Fprintf(c.out, "xray process %d not found (already stopped).\n", pid)
		return c.removePID()
	}
	if !c.owned(pid) {
		fmt.Fprintf(c.errOut, "Refusing to stop PID %d: it is not the xray process started by dots-proxy.\nRemoving stale PID file without stopping that process.\n", pid)
		return c.removePID()
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			fmt.Fprintf(c.out, "xray process %d not found (already stopped).\n", pid)
			return c.removePID()
		}
		return errors.New("Could not stop xray process; PID file retained.")
	}
	deadline := time.Now().Add(5 * time.Second)
	for pidRunning(pid) {
		if !c.owned(pid) {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("xray did not exit after SIGTERM; PID file retained.")
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(c.out, "xray stopped (PID %d).\n", pid)
	return c.removePID()
}
func (c *controller) requireRunning() error {
	pid, err := c.readPID()
	if err != nil {
		return err
	}
	if pid > 0 && pidRunning(pid) && c.owned(pid) {
		return nil
	}
	return errors.New("xray proxy is not running. Start it first: dots-proxy")
}
