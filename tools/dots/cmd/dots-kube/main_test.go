package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testBinary string

func TestMain(m *testing.M) {
	if os.Getenv("DOTS_KUBE_LOG_FIXTURE") == "1" && filepath.Base(os.Args[0]) == "kubectl" {
		runLogFixture()
		return
	}
	directory, err := os.MkdirTemp("", "dots-kube-binary-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		os.RemoveAll(directory)
		os.Exit(1)
	}
	testBinary = filepath.Join(directory, "dots-kube")
	command := exec.Command("go", "-C", root, "build", "-trimpath", "-o", testBinary, "./cmd/dots-kube")
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(directory)
		os.Exit(1)
	}
	status := m.Run()
	os.RemoveAll(directory)
	os.Exit(status)
}
func execute(t *testing.T, env []string, binary string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = env
	command.Dir = t.TempDir()
	var out, diagnostic bytes.Buffer
	command.Stdout = &out
	command.Stderr = &diagnostic
	err := command.Run()
	status := 0
	if err != nil {
		if process, ok := err.(*exec.ExitError); ok {
			status = process.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return out.String(), diagnostic.String(), status
}
func TestStandaloneCanConfigureAndInstallAliasesWithoutARepository(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "commands")
	fakeBin := filepath.Join(home, "dependency-bin")
	if err := os.MkdirAll(fakeBin, 0755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "provided.yaml")
	document := `apiVersion: v1
kind: Config
current-context: original
clusters:
- name: local-fixture
  cluster: {server: https://127.0.0.1:1}
users:
- name: fixture-user
  user: {token: PRIVATE-FIXTURE-TOKEN}
contexts:
- name: original
  context: {cluster: local-fixture, user: fixture-user, namespace: original}
`
	if err := os.WriteFile(config, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
printf '%s\n' "$KUBECONFIG" >> "$CALLS"
case " $* " in
 *' get pods '*) printf 'fixture pods\n' ;;
 *) printf 'unexpected kubectl call\n' >&2; exit 97 ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "kubectl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "PATH=" + fakeBin, "KUBECONFIG=" + config, "NO_COLOR=1", "CALLS=" + filepath.Join(home, "calls")}
	out, diagnostic, status := execute(t, env, testBinary, "setup", "stage", "--context", "original", "--namespace", "stage-ns", "--bin-dir", bin, "--yes")
	if status != 0 || strings.Contains(out+diagnostic, "PRIVATE-FIXTURE-TOKEN") {
		t.Fatalf("setup %d %q %q", status, out, diagnostic)
	}
	canonical := filepath.Join(bin, "dots-kube")
	if info, err := os.Stat(canonical); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("installed binary %v %v", info, err)
	}
	for _, name := range []string{"kpdev-rollout-restart", "kpstage-rollout-restart", "kppreprod-rollout-restart", "kpprod-rollout-restart"} {
		target, err := os.Readlink(filepath.Join(bin, name))
		if err != nil || target != "dots-kube" {
			t.Fatalf("missing standalone rollout alias %s: %q %v", name, target, err)
		}
	}
	out, diagnostic, status = execute(t, env, filepath.Join(bin, "kpstage"), "get", "pods")
	if status != 0 || out != "fixture pods\n" || !strings.Contains(diagnostic, "context=dots-stage namespace=stage-ns") {
		t.Fatalf("alias %d %q %q", status, out, diagnostic)
	}
	out, diagnostic, status = execute(t, env, canonical, "config", "--json")
	if status != 0 || !strings.Contains(out, `"current_context": "original"`) || strings.Contains(out, "PRIVATE-FIXTURE-TOKEN") {
		t.Fatalf("view %d %q %q", status, out, diagnostic)
	}
	_, diagnostic, status = execute(t, env, canonical, "install-aliases", "--bin-dir", bin, "--yes")
	if status != 0 {
		t.Fatalf("idempotency %d %q", status, diagnostic)
	}
	if err := os.Remove(filepath.Join(bin, "kpprod")); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("foreign command")
	if err := os.WriteFile(filepath.Join(bin, "kpprod"), foreign, 0755); err != nil {
		t.Fatal(err)
	}
	_, diagnostic, status = execute(t, env, canonical, "install-aliases", "--bin-dir", bin, "--yes")
	data, _ := os.ReadFile(filepath.Join(bin, "kpprod"))
	if status == 0 || string(data) != string(foreign) || !strings.Contains(diagnostic, "preserved") {
		t.Fatalf("collision %d %q", status, diagnostic)
	}
}
func TestStandaloneHelpPreviewAndVersionNeedNoExternalTools(t *testing.T) {
	env := []string{"HOME=" + t.TempDir(), "PATH=" + t.TempDir(), "NO_COLOR=1"}
	for _, args := range [][]string{{"--help"}, {"version"}, {"preview", "logs", "--plain"}, {"config", "--json"}} {
		out, diagnostic, status := execute(t, env, testBinary, args...)
		if status != 0 || out == "" || diagnostic != "" || strings.Contains(out, "\x1b") {
			t.Fatalf("%v %d %q %q", args, status, out, diagnostic)
		}
	}
}
