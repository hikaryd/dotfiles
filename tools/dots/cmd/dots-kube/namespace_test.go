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

func TestNamespacePickerReadsClusterAndSavesOnlyAfterConfirmation(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if err := os.Symlink(exe, filepath.Join(bin, "kubectl")); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "provided.yaml")
	document := "apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster: {server: https://127.0.0.1:1}\ncontexts:\n- name: original\n  context: {cluster: fixture, namespace: default}\n"
	os.WriteFile(config, []byte(document), 0600)
	env := []string{"HOME=" + home, "PATH=" + bin, "DOTS_KUBE_LOG_FIXTURE=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
	_, diagnostic, status := execute(t, env, testBinary, "setup", "stage", "--kubeconfig", config, "--context", "original", "--namespace", "default", "--yes", "--skip-aliases")
	if status != 0 {
		t.Fatalf("initial setup %d %q", status, diagnostic)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, testBinary, "namespace", "stage")
	command.Env = env
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 28, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer command.Process.Kill()
	capture := &terminalCapture{}
	go func() {
		buffer := make([]byte, 8192)
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
	awaitScreen(t, capture, "Namespace из кластера")
	awaitScreen(t, capture, "application")
	terminal.Write([]byte("\r"))
	awaitScreen(t, capture, "Сохранить namespace")
	out, diagnostic, status := execute(t, env, testBinary, "config", "--json")
	if status != 0 || !strings.Contains(out, `"namespace": "default"`) {
		t.Fatalf("selection wrote before confirmation %d %q %q", status, out, diagnostic)
	}
	terminal.Write([]byte("\r"))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, status = execute(t, env, testBinary, "config", "--json")
	if status != 0 || !strings.Contains(out, `"namespace": "application"`) {
		t.Fatalf("namespace not saved %d %q %q", status, out, diagnostic)
	}
	original, _ := os.ReadFile(config)
	if string(original) != document {
		t.Fatal("changed original kubeconfig")
	}
}

func TestSetupCanChooseNamespaceFromClusterWithoutTypingItsName(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0755)
	exe, _ := os.Executable()
	os.Symlink(exe, filepath.Join(bin, "kubectl"))
	config := filepath.Join(home, "provided.yaml")
	document := "apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster: {server: https://127.0.0.1:1}\ncontexts:\n- name: original\n  context: {cluster: fixture}\n"
	os.WriteFile(config, []byte(document), 0600)
	env := []string{"HOME=" + home, "PATH=" + bin, "DOTS_KUBE_LOG_FIXTURE=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, testBinary, "setup", "stage", "--kubeconfig", config, "--skip-aliases")
	command.Env = env
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 28, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer command.Process.Kill()
	capture := &terminalCapture{}
	go func() {
		buffer := make([]byte, 8192)
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
	awaitScreen(t, capture, "STAGE · Контекст")
	terminal.Write([]byte("\r"))
	awaitScreen(t, capture, "STAGE · Namespace")
	terminal.Write([]byte("draft-ns"))
	terminal.Write([]byte{15}) // Ctrl+O opens the namespace lookup without F-keys.
	awaitScreen(t, capture, "Namespace из кластера")
	offset := len(capture.String())
	terminal.Write([]byte{27})
	awaitScreenAfter(t, capture, offset, "Namespace › draft-ns")
	offset = len(capture.String())
	terminal.Write([]byte{15})
	awaitScreenAfter(t, capture, offset, "Namespace из кластера")
	awaitScreen(t, capture, "application")
	terminal.Write([]byte("\r"))
	awaitScreen(t, capture, "3/3 · Проверка")
	terminal.Write([]byte("\r"))
	if err := command.Wait(); err != nil {
		t.Fatalf("setup %v %q", err, capture.String())
	}
	out, diagnostic, status := execute(t, env, testBinary, "config", "--json")
	if status != 0 || !strings.Contains(out, `"namespace": "application"`) {
		t.Fatalf("discovered namespace %d %q %q", status, out, diagnostic)
	}
}

func assertRepeatedNamespace(t *testing.T, home, config string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, testBinary, "setup", "stage", "--kubeconfig", config, "--skip-aliases")
	command.Env = []string{"HOME=" + home, "PATH=" + t.TempDir(), "NO_COLOR=1", "TERM=xterm-256color"}
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 28, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer command.Process.Kill()
	capture := &terminalCapture{}
	go func() {
		buffer := make([]byte, 8192)
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
	awaitScreen(t, capture, "STAGE · Контекст")
	terminal.Write([]byte("\r"))
	awaitScreen(t, capture, "Namespace › stage-ns")
	// Nothing is saved by this repeat run: a previously confirmed namespace must
	// already be offered before any confirmation or cluster request.
	command.Process.Kill()
	command.Wait()
}
