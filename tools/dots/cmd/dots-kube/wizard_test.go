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

func TestWizardDiscoversMultipleFilesAndRemembersIndependentEnvironments(t *testing.T) {
	for _, layout := range []struct {
		name       string
		rows, cols uint16
	}{{"wide", 32, 110}, {"narrow", 24, 60}} {
		t.Run(layout.name, func(t *testing.T) {
			home := t.TempDir()
			directory := filepath.Join(home, ".kube", "config")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			document := func(server string) string {
				return "apiVersion: v1\nkind: Config\ncurrent-context: original\nclusters:\n- name: shared-name\n  cluster: {server: " + server + "}\nusers:\n- name: shared-user\n  user: {token: WIZARD-PRIVATE-TOKEN}\ncontexts:\n- name: original\n  context: {cluster: shared-name, user: shared-user, namespace: application}\n"
			}
			stage, prod := filepath.Join(directory, "stage.yaml"), filepath.Join(directory, "prod.yaml")
			stageDoc, prodDoc := document("https://127.0.0.1:1"), document("https://127.0.0.1:2")
			os.WriteFile(stage, []byte(stageDoc), 0600)
			os.WriteFile(prod, []byte(prodDoc), 0600)
			env := []string{"HOME=" + home, "PATH=" + t.TempDir(), "KUBECONFIG=" + directory, "NO_COLOR=1", "TERM=xterm-256color"}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, testBinary, "setup", "--skip-aliases")
			command.Env = env
			terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: layout.rows, Cols: layout.cols})
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
			awaitScreen(t, capture, "1/3 · Файлы")
			awaitScreen(t, capture, "stage.yaml")
			awaitScreen(t, capture, "prod.yaml")
			terminal.Write([]byte("\x01\r"))
			awaitScreen(t, capture, "2/3 · Окружения")
			terminal.Write([]byte("\x1b[B\r"))
			awaitScreen(t, capture, "STAGE · Контекст")
			terminal.Write([]byte("stage.yaml\r"))
			awaitScreen(t, capture, "STAGE · Namespace")
			beforeEnvironment := len(capture.String())
			terminal.Write([]byte("\x0cstage-ns\r"))
			awaitScreenAfter(t, capture, beforeEnvironment, "2/3 · Окружения")
			beforeProd := len(capture.String())
			terminal.Write([]byte("\x1b[B\x1b[B\x1b[B\r"))
			awaitScreen(t, capture, "PROD · Контекст")
			time.Sleep(30 * time.Millisecond)
			fresh := capture.String()[beforeProd:]
			if !strings.Contains(fresh, "stage.yaml") || !strings.Contains(fresh, "prod.yaml") {
				t.Fatalf("previous search leaked into next environment: %q", fresh)
			}
			terminal.Write([]byte("prod.yaml\r"))
			awaitScreen(t, capture, "PROD · Namespace")
			terminal.Write([]byte("\x0cprod-ns\r"))
			awaitScreen(t, capture, "prod-ns")
			terminal.Write([]byte("\x1b[F\r"))
			awaitScreen(t, capture, "3/3 · Проверка")
			// Esc must return to environment choices, not discard the plan or write it.
			terminal.Write([]byte{27})
			time.Sleep(100 * time.Millisecond)
			if _, err := os.Stat(filepath.Join(home, ".config", "dots-kube", "sources.yaml")); !os.IsNotExist(err) {
				t.Fatal("back wrote settings before confirmation")
			}
			terminal.Write([]byte("\x1b[F\r"))
			time.Sleep(100 * time.Millisecond)
			terminal.Write([]byte("\r"))
			if err := command.Wait(); err != nil {
				t.Fatalf("wizard %v %q", err, capture.String())
			}
			if strings.Contains(capture.String(), "WIZARD-PRIVATE-TOKEN") || strings.Contains(capture.String(), "export KUBECONFIG") {
				t.Fatal("wizard exposed credentials or needed shell exports")
			}
			first, _ := os.ReadFile(stage)
			second, _ := os.ReadFile(prod)
			if string(first) != stageDoc || string(second) != prodDoc {
				t.Fatal("wizard modified supplied files")
			}
			out, diagnostic, status := execute(t, env, testBinary, "config", "--json")
			if status != 0 || diagnostic != "" || !strings.Contains(out, `"namespace": "stage-ns"`) || !strings.Contains(out, `"namespace": "prod-ns"`) {
				t.Fatalf("saved config %d %q %q", status, out, diagnostic)
			}
		})
	}
}
