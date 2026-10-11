package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestSinglePodAliasUsesReadablePausableLogsWithoutGum(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(bin, "kubectl")); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(bin, "kpstage-logs")
	if err := os.Symlink(testBinary, alias); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(config, []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster: {server: https://127.0.0.1:1}\ncontexts:\n- name: dots-stage\n  context: {cluster: fixture, namespace: app}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, alias, "api")
	command.Env = []string{"HOME=" + home, "PATH=" + bin, "KUBECONFIG=" + config, "DOTS_KUBE_LOG_FIXTURE=1", "DOTS_KUBE_LOG_JSON=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer command.Process.Kill()
	capture := &terminalCapture{}
	go func() {
		buffer := make([]byte, 32768)
		for {
			n, err := terminal.Read(buffer)
			if n > 0 {
				capture.Write(buffer[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	awaitScreen(t, capture, "choose a pod")
	terminal.Write([]byte("\r"))
	awaitScreen(t, capture, "Pod logs · api")
	awaitScreen(t, capture, "readable second line")
	initial := capture.String()
	if strings.Contains(initial, "DETAIL_SENTINEL") || !strings.Contains(initial, "readable first line") {
		t.Fatalf("metadata polluted reading view: %q", initial)
	}
	terminal.Write([]byte(" "))
	awaitScreen(t, capture, "PAUSED")
	terminal.Write([]byte("\r"))
	awaitScreen(t, capture, "DETAIL_SENTINEL")
	terminal.Write([]byte("\r"))
	time.Sleep(80 * time.Millisecond)
	// Bracketed paste must become search text, never execute q/p as hotkeys.
	terminal.Write([]byte("\x1b[200~q p\x1b[201~"))
	awaitScreen(t, capture, "Search › q p")
	terminal.Write([]byte{27})
	time.Sleep(100 * time.Millisecond)
	terminal.Write([]byte("q"))
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("single-pod viewer did not exit")
	}
}
