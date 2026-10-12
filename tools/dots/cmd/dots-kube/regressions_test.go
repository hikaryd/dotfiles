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

// Exercise the public CLI in a real terminal. Only kubectl is a fixture; the
// wizard, selectors, dispatch, sources and native log dashboard remain real.
func startUITest(t *testing.T, env []string, args ...string) (*os.File, *terminalCapture, *exec.Cmd) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	command := exec.CommandContext(ctx, testBinary, args...)
	command.Env = env
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 32, Cols: 110})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); terminal.Close(); cancel() })
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
	return terminal, capture, command
}

func uiStep(t *testing.T, terminal *os.File, capture *terminalCapture, keys, next string) {
	t.Helper()
	offset := len(capture.String())
	if _, err := terminal.Write([]byte(keys)); err != nil {
		t.Fatal(err)
	}
	awaitScreenAfter(t, capture, offset, next)
}

func TestWizardNamespaceDiscoverySwitchesSavedSourceWithEnvironment(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		name := "saved-source"
		if explicit {
			name = "explicit-source"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".kube", "configs")
			bin := filepath.Join(home, "bin")
			for _, p := range []string{dir, bin} {
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			stage, preprod := filepath.Join(dir, "stage.yaml"), filepath.Join(dir, "preprod.yaml")
			for _, file := range []string{stage, preprod} {
				document := "apiVersion: v1\nkind: Config\nclusters:\n- name: shared\n  cluster: {server: https://127.0.0.1:1}\nusers:\n- name: user\n  user: {}\ncontexts:\n- name: original\n  context: {cluster: shared, user: user, namespace: initial}\n"
				if err := os.WriteFile(file, []byte(document), 0600); err != nil {
					t.Fatal(err)
				}
			}
			script := `#!/bin/sh
printf '%s|%s\n' "$KUBECONFIG" "$*" >> "$CALLS"
case " $* " in
 *' get namespaces '*)
  case "$KUBECONFIG" in
   "$STAGE") printf '{"items":[{"metadata":{"name":"stage-only"}}]}\n' ;;
   "$PREPROD") printf '{"items":[{"metadata":{"name":"preprod-only"}}]}\n' ;;
   *) exit 97 ;;
  esac ;;
 *) exit 97 ;;
esac
`
			if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			calls := filepath.Join(home, "calls")
			env := []string{"HOME=" + home, "PATH=" + bin, "STAGE=" + stage, "PREPROD=" + preprod, "CALLS=" + calls, "NO_COLOR=1", "TERM=xterm-256color"}
			for _, profile := range []struct{ env, file string }{{"stage", stage}, {"preprod", preprod}} {
				out, diagnostic, status := execute(t, env, testBinary, "setup", profile.env, "--kubeconfig", profile.file, "--context", "original", "--namespace", profile.env+"-only", "--skip-aliases", "--yes")
				if status != 0 {
					t.Fatalf("seed %d %q %q", status, out, diagnostic)
				}
			}
			terminal, capture, command := startUITest(t, env, "setup", "--skip-aliases")
			awaitScreen(t, capture, "1/3 · Файлы")
			uiStep(t, terminal, capture, "\x01\r", "2/3 · Окружения")
			uiStep(t, terminal, capture, "\x1b[B\x1b[B\r", "PREPROD · Контекст")
			uiStep(t, terminal, capture, "\r", "PREPROD · Namespace")
			uiStep(t, terminal, capture, "\x0f", "preprod-only")
			uiStep(t, terminal, capture, "\r", "2/3 · Окружения")
			uiStep(t, terminal, capture, "\x1b[B\r", "STAGE · Контекст")
			keys := "\r"
			if explicit {
				keys = "stage.yaml\r"
			}
			uiStep(t, terminal, capture, keys, "STAGE · Namespace")
			offset := len(capture.String())
			terminal.Write([]byte{15})
			awaitScreenAfter(t, capture, offset, "Namespace из кластера")
			data, _ := os.ReadFile(calls)
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) != 2 || !strings.HasPrefix(lines[0], preprod+"|") || !strings.HasPrefix(lines[1], stage+"|") || !strings.Contains(capture.String()[offset:], "stage-only") {
				t.Fatalf("stage reused preprod namespaces: calls=%q screen=%q", data, capture.String()[offset:])
			}
			uiStep(t, terminal, capture, "\r", "2/3 · Окружения")
			uiStep(t, terminal, capture, "\x1b[F\r", "3/3 · Проверка")
			terminal.Write([]byte("\r"))
			if err := command.Wait(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMultiselectCanSubmitMarkedPodsHiddenBySearch(t *testing.T) {
	env, calls := podPickerFixture(t)
	terminal, capture, command := startUITest(t, env, "kpstage-logs-multi")
	awaitScreen(t, capture, "choose pods")
	uiStep(t, terminal, capture, "api ", "1 selected")
	uiStep(t, terminal, capture, "\x0cnot-a-pod", "No matching resources")
	uiStep(t, terminal, capture, "\r", "log fixture")
	data, _ := os.ReadFile(calls)
	if !strings.Contains(string(data), "pod/api-one") || strings.Contains(string(data), "pod/worker-two") {
		t.Fatalf("hidden selection lost: %q", data)
	}
	terminal.Write([]byte("q"))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestWizardAcceptsRelativeSourcePaths(t *testing.T) {
	env, _ := podPickerFixture(t)
	var config string
	for _, entry := range env {
		if strings.HasPrefix(entry, "KUBECONFIG=") {
			config = strings.TrimPrefix(entry, "KUBECONFIG=")
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, config)
	if err != nil {
		t.Fatal(err)
	}
	terminal, capture, command := startUITest(t, env, "setup", "stage", "--kubeconfig", relative, "--skip-aliases")
	awaitScreen(t, capture, "STAGE · Контекст")
	uiStep(t, terminal, capture, "\r", "STAGE · Namespace")
	uiStep(t, terminal, capture, "\r", "3/3 · Проверка")
	terminal.Write([]byte("\r"))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, status := execute(t, env, testBinary, "config", "--json")
	if status != 0 || !strings.Contains(out, `"namespace": "application"`) {
		t.Fatalf("relative setup saved unusable sources: %d %q %q", status, out, diagnostic)
	}
}

func TestWizardRemembersSelectedContextWithinOneSourceFile(t *testing.T) {
	env, _ := podPickerFixture(t)
	var config string
	for _, entry := range env {
		if strings.HasPrefix(entry, "KUBECONFIG=") {
			config = strings.TrimPrefix(entry, "KUBECONFIG=")
		}
	}
	data, _ := os.ReadFile(config)
	data = append(data, []byte("- name: stage-source\n  context: {cluster: fixture, namespace: application}\n")...)
	os.WriteFile(config, data, 0600)
	out, diagnostic, status := execute(t, env, testBinary, "setup", "stage", "--kubeconfig", config, "--context", "stage-source", "--namespace", "application", "--skip-aliases", "--yes")
	if status != 0 {
		t.Fatalf("seed %d %q %q", status, out, diagnostic)
	}
	terminal, capture, command := startUITest(t, env, "setup", "stage", "--kubeconfig", config, "--skip-aliases")
	awaitScreen(t, capture, "STAGE · Контекст")
	uiStep(t, terminal, capture, "\r", "STAGE · Namespace")
	if !strings.Contains(capture.String(), "context: stage-source") {
		t.Fatalf("saved context replaced by first row: %q", capture.String())
	}
	uiStep(t, terminal, capture, "\r", "3/3 · Проверка")
	terminal.Write([]byte("\r"))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	// Changing just namespace must not erase which source context was selected.
	terminal, capture, command = startUITest(t, env, "namespace", "stage")
	awaitScreen(t, capture, "Namespace из кластера")
	uiStep(t, terminal, capture, "\r", "Сохранить namespace")
	terminal.Write([]byte("\r"))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	terminal, capture, command = startUITest(t, env, "setup", "stage", "--kubeconfig", config, "--skip-aliases")
	awaitScreen(t, capture, "STAGE · Контекст")
	uiStep(t, terminal, capture, "\r", "STAGE · Namespace")
	if !strings.Contains(capture.String(), "context: stage-source") {
		t.Fatal("namespace change erased source context")
	}
	command.Process.Kill()
	command.Wait()
}

func TestWideMultiLogsKeepNamesAndEnterOpensLiveDetails(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0700)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	os.Symlink(exe, filepath.Join(bin, "kubectl"))
	env := []string{"HOME=" + home, "PATH=" + bin, "DOTS_KUBE_LOG_FIXTURE=1", "DOTS_KUBE_LOG_JSON=1", "DOTS_KUBE_LOG_LAYOUT=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
	pods := []string{"application-api-7755446644-aaaaa", "application-worker-6655888877-bbbbb"}
	terminal, capture, command := startUITest(t, env, "kube-log-watch", "--context", "dots-stage", "--namespace", "app", "--environment", "stage", "--pod", pods[0], "--pod", pods[1])
	if err := pty.Setsize(terminal, &pty.Winsize{Rows: 32, Cols: 180}); err != nil {
		t.Fatal(err)
	}
	awaitScreen(t, capture, "readable first line")
	time.Sleep(150 * time.Millisecond)
	data := capture.String()
	for _, value := range []string{"POD", "CONTAINER", "LEVEL", pods[0], pods[1], "application-long-container"} {
		if !strings.Contains(data, value) {
			t.Fatalf("wide logs clipped useful metadata %q: %q", value, data)
		}
	}
	if !strings.Contains(data, "hidden_detail") || !strings.Contains(data, "payload") {
		t.Fatal("full JSON metadata was silently hidden in the default list")
	}
	uiStep(t, terminal, capture, "\r", "DETAIL  pod=")
	awaitScreen(t, capture, "PAUSED")
	terminal.Write([]byte("q"))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeExecScaleAndJobWorkflows(t *testing.T) {
	for _, action := range []string{"exec", "scale", "jobs"} {
		t.Run(action, func(t *testing.T) {
			env, calls := podPickerFixture(t)
			var bin string
			for _, entry := range env {
				if strings.HasPrefix(entry, "PATH=") {
					bin = strings.TrimPrefix(entry, "PATH=")
				}
			}
			script := `#!/bin/sh
printf '%s|%s\n' "$KUBECONFIG" "$*" >> "$CALLS"
case " $* " in
 *' get pods,deployments,statefulsets,daemonsets '*) printf '{"items":[{"kind":"Deployment","metadata":{"name":"api"}}]}\n' ;;
 *' get deployments,statefulsets '*) printf '{"items":[{"kind":"Deployment","metadata":{"name":"api"},"spec":{"replicas":2},"status":{"replicas":2,"readyReplicas":2,"availableReplicas":2}}]}\n' ;;
 *' get jobs '*) printf '{"items":[{"metadata":{"name":"nightly","creationTimestamp":"2026-01-01T00:00:00Z"},"status":{"succeeded":1}}]}\n' ;;
 *' exec '*) printf 'EXEC_OK\n' ;;
 *' scale '*) printf 'SCALE_OK\n' ;;
 *' logs '*) printf 'JOBS_OK\n' ;;
 *) exit 97 ;;
esac
`
			os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0755)
			terminal, capture, command := startUITest(t, env, "kpstage-"+action)
			expected := ""
			switch action {
			case "exec":
				awaitScreen(t, capture, "choose where to execute")
				uiStep(t, terminal, capture, "\r", "what to execute")
				uiStep(t, terminal, capture, "\r", "working directory")
				terminal.Write([]byte("\r"))
				expected = "exec -it deployment/api -- bash"
			case "scale":
				awaitScreen(t, capture, "scale workload")
				uiStep(t, terminal, capture, "\r", "Desired replicas")
				terminal.Write([]byte("\x0c3\r"))
				expected = "scale deployment/api --replicas=3"
			case "jobs":
				awaitScreen(t, capture, "Jobs newest first")
				terminal.Write([]byte("\r"))
				expected = "logs -f -l job-name=nightly"
			}
			if err := command.Wait(); err != nil {
				t.Fatalf("%s failed: %v screen=%q", action, err, capture.String())
			}
			data, _ := os.ReadFile(calls)
			if !strings.Contains(string(data), expected) || strings.Contains(string(data), "--context=dots-prod") {
				t.Fatalf("wrong workflow target/command: %q", data)
			}
		})
	}
}

func TestPodAnalyticsCancelsDuringInitialFetch(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0700)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(bin, "kubectl")); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(home, "ready")
	env := []string{"HOME=" + home, "PATH=" + bin, "DOTS_KUBE_LOG_FIXTURE=1", "DOTS_KUBE_BLOCK_INITIAL=1", "DOTS_KUBE_INITIAL_READY=" + ready, "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
	terminal, _, command := startUITest(t, env, "kube-pod-analyze", "--context", "dots-stage", "--namespace", "app", "--environment", "stage", "--pod", "api", "--processes=false")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture never entered initial fetch")
		}
		time.Sleep(10 * time.Millisecond)
	}
	terminal.Write([]byte("q"))
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("q did not cancel initial kubectl fetch")
	}
}

func podPickerFixture(t *testing.T) ([]string, string) {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "provided.yaml")
	document := "apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster: {server: https://127.0.0.1:1}\ncontexts:\n- name: dots-stage\n  context: {cluster: fixture, namespace: application}\n"
	os.WriteFile(config, []byte(document), 0600)
	script := `#!/bin/sh
printf '%s|%s\n' "$KUBECONFIG" "$*" >> "$CALLS"
case " $* " in
 *' get pods '*) printf '{"items":[{"metadata":{"name":"api-one"},"status":{"phase":"Running"}},{"metadata":{"name":"worker-two"},"status":{"phase":"Running"}},{"metadata":{"name":"unselected"},"status":{"phase":"Running"}}]}\n' ;;
 *' logs '*) printf 'INFO log fixture\n' ;;
 *' get namespaces '*) printf '{"items":[{"metadata":{"name":"application"}}]}\n' ;;
 *) exit 97 ;;
esac
`
	os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0755)
	calls := filepath.Join(home, "calls")
	env := []string{"HOME=" + home, "PATH=" + bin, "KUBECONFIG=" + config, "CALLS=" + calls, "NO_COLOR=1", "TERM=xterm-256color"}
	return env, calls
}

func TestLogsMultiWithoutPatternOffersSearchablePodMultiselect(t *testing.T) {
	env, calls := podPickerFixture(t)
	terminal, capture, command := startUITest(t, env, "kpstage-logs-multi")
	awaitScreen(t, capture, "choose pods")
	awaitScreen(t, capture, "api-one")
	uiStep(t, terminal, capture, "api ", "1 selected")
	uiStep(t, terminal, capture, "\x0cworker ", "2 selected")
	terminal.Write([]byte("\r"))
	awaitScreen(t, capture, "log fixture")
	time.Sleep(50 * time.Millisecond)
	data, _ := os.ReadFile(calls)
	if !strings.Contains(string(data), "pod/api-one") || !strings.Contains(string(data), "pod/worker-two") || strings.Contains(string(data), "pod/unselected") {
		t.Fatalf("wrong selected log streams: %q", data)
	}
	terminal.Write([]byte("q"))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}
