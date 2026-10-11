package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type terminalCapture struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (c *terminalCapture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data.Write(data)
}
func (c *terminalCapture) String() string { c.mu.Lock(); defer c.mu.Unlock(); return c.data.String() }
func awaitScreen(t *testing.T, capture *terminalCapture, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(capture.String(), text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("screen did not show %q: %q", text, capture.String())
}
func awaitScreenAfter(t *testing.T, capture *terminalCapture, offset int, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data := capture.String()
		if len(data) >= offset && strings.Contains(data[offset:], text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("new screen did not show %q", text)
}

func TestNativeSetupWizardNeedsNoGumAndRestoresTerminal(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		cancel, missing bool
	}{{name: "configure"}, {name: "cancel", cancel: true}, {name: "missing-namespace", missing: true}} {
		cancelWizard := scenario.cancel
		t.Run(scenario.name, func(t *testing.T) {
			home := t.TempDir()
			config := filepath.Join(home, ".kube", "config")
			if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
				t.Fatal(err)
			}
			document := "apiVersion: v1\nkind: Config\ncurrent-context: original\nclusters:\n- name: fixture\n  cluster: {server: https://127.0.0.1:1}\ncontexts:\n- name: original\n  context: {cluster: fixture, namespace: original}\n"
			if scenario.missing {
				document = strings.Replace(document, ", namespace: original", "", 1)
			}
			if err := os.WriteFile(config, []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			master, slave, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			defer slave.Close()
			pty.Setsize(master, &pty.Winsize{Rows: 28, Cols: 100})
			before, err := term.GetState(int(master.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, testBinary, "setup", "stage", "--skip-aliases")
			command.Env = []string{"HOME=" + home, "PATH=" + t.TempDir(), "NO_COLOR=1", "TERM=xterm-256color"}
			command.Stdin = slave
			command.Stdout = slave
			command.Stderr = slave
			command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			defer command.Process.Kill()
			capture := &terminalCapture{}
			go func() {
				buffer := make([]byte, 8192)
				for {
					n, err := master.Read(buffer)
					if n > 0 {
						capture.Write(buffer[:n])
					}
					if err != nil {
						return
					}
				}
			}()
			awaitScreen(t, capture, "1/3 · Файлы")
			if cancelWizard {
				master.Write([]byte{27})
			} else {
				// Ctrl+O must also open the file browser from a searchable selector.
				master.Write([]byte{15})
				awaitScreen(t, capture, "1/3 · Выбор файла")
				beforeFiles := len(capture.String())
				master.Write([]byte("\x1b[B\r"))
				awaitScreenAfter(t, capture, beforeFiles, "1/3 · Файлы")
				master.Write([]byte("\r"))
				awaitScreen(t, capture, "STAGE · Контекст")
				master.Write([]byte("\r"))
				awaitScreen(t, capture, "STAGE · Namespace")
				if scenario.missing {
					master.Write([]byte("\r"))
					awaitScreen(t, capture, "Укажите namespace")
				}
				master.Write([]byte("\x0cstage-ns\r"))
				awaitScreen(t, capture, "3/3 · Проверка")
				master.Write([]byte("\r"))
			}
			err = command.Wait()
			if cancelWizard {
				process, ok := err.(*exec.ExitError)
				if !ok || process.ExitCode() != 130 {
					t.Fatalf("cancel %v %q", err, capture.String())
				}
			} else if err != nil {
				t.Fatalf("wizard %v %q", err, capture.String())
			}
			after, err := term.GetState(int(master.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("terminal was not restored")
			}
			data, _ := os.ReadFile(config)
			if string(data) != document {
				t.Fatal("wizard changed the supplied kubeconfig")
			}
			if !cancelWizard {
				env := []string{"HOME=" + home, "PATH=" + t.TempDir(), "NO_COLOR=1"}
				out, diagnostic, status := execute(t, env, testBinary, "config", "--json")
				if status != 0 || !strings.Contains(out, `"namespace": "stage-ns"`) {
					t.Fatalf("saved config %d %q %q", status, out, diagnostic)
				}
				assertRepeatedNamespace(t, home, config)
			}
		})
	}
}
