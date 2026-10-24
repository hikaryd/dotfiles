package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestConfigReportsBindingsWithoutCredentialsOrClusterCalls(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", anyCall("unexpected", 99))
	f.fake("zsh", anyCall("unexpected", 99))
	out, diagnostic, status := f.run("", "config", "--json")
	if status != 0 || diagnostic != "" || !strings.Contains(out, `"current_context": "unrelated"`) || !strings.Contains(out, `"context": "dots-stage"`) || strings.Contains(out, "fixture-secret") || strings.Contains(out, "token") || len(f.calls()) != 0 {
		t.Fatalf("%d %q %q %+v", status, out, diagnostic, f.calls())
	}
}

func TestSetupBindsExistingContextIdempotentlyWithoutChangingGlobalContext(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact([]string{"--context=dots-stage", "--namespace=new-stage", "get", "pods"}, "pods\n", 0))
	args := []string{"setup", "stage", "--context", "unrelated", "--namespace", "new-stage", "--yes", "--skip-aliases"}
	out, diagnostic, status := f.run("", args...)
	if status != 0 || strings.Contains(out+diagnostic, "fixture-secret") {
		t.Fatalf("setup %d %q %q", status, out, diagnostic)
	}
	first, _ := os.ReadFile(f.config)
	if !strings.Contains(string(first), "fixture-secret") || !strings.Contains(string(first), "current-context: unrelated") {
		t.Fatal("setup discarded credentials or changed current-context")
	}
	_, diagnostic, status = f.run("", args...)
	second, _ := os.ReadFile(f.config)
	if status != 0 || string(first) != string(second) {
		t.Fatalf("idempotency %d %q", status, diagnostic)
	}
	out, diagnostic, status = f.run("", "config", "--json")
	if status != 0 || !strings.Contains(out, `"namespace": "new-stage"`) || !strings.Contains(out, `"current_context": "unrelated"`) {
		t.Fatalf("config %d %q %q", status, out, diagnostic)
	}
	out, diagnostic, status = f.run("", "kpstage", "get", "pods")
	if status != 0 || out != "pods\n" {
		t.Fatalf("alias %d %q %q", status, out, diagnostic)
	}
	info, _ := os.Stat(f.config)
	if info.Mode().Perm() != 0600 {
		t.Fatal("kubeconfig is not private")
	}
}

func TestSetupImportsOnAnEmptyMachineWithoutPrintingCredentials(t *testing.T) {
	f := newFixture(t)
	if err := os.Remove(f.config); err != nil {
		t.Fatal(err)
	}
	incoming := filepath.Join(f.home, "provided.yaml")
	document := `apiVersion: v1
kind: Config
current-context: incoming
clusters:
- name: fixture
  cluster: {server: https://127.0.0.1:1}
users:
- name: fixture-user
  user: {token: PRIVATE-IMPORT-TOKEN}
contexts:
- name: incoming
  context: {cluster: fixture, user: fixture-user, namespace: app}
`
	writeConfig(t, incoming, document)
	f.fake("kubectl", exact([]string{"config", "view", "--flatten", "--raw", "-o", "yaml"}, document, 0), exact([]string{"--context=dots-stage", "--namespace=app", "get", "pods"}, "pods\n", 0))
	out, diagnostic, status := f.run("", "setup", "stage", "--import", incoming, "--context", "incoming", "--yes", "--skip-aliases")
	if status != 0 || strings.Contains(out+diagnostic, "PRIVATE-IMPORT-TOKEN") {
		t.Fatalf("import %d %q %q", status, out, diagnostic)
	}
	data, _ := os.ReadFile(f.config)
	if !strings.Contains(string(data), "PRIVATE-IMPORT-TOKEN") {
		t.Fatal("import lost credentials")
	}
	out, diagnostic, status = f.run("", "config", "--json")
	if status != 0 || !strings.Contains(out, `"current_context": ""`) || !strings.Contains(out, `"context": "dots-stage"`) || strings.Contains(out, "PRIVATE-IMPORT-TOKEN") {
		t.Fatalf("report %d %q %q", status, out, diagnostic)
	}
	_, diagnostic, status = f.run("", "kpstage", "get", "pods")
	if status != 0 {
		t.Fatalf("alias %d %q", status, diagnostic)
	}
	calls := f.calls()
	if calls[0].Kubeconfig != f.config+string(os.PathListSeparator)+incoming {
		t.Fatalf("merge chain %+v", calls[0])
	}
}

func TestPreviewIsIndependentOfKubeconfigAndRespectsPlainOutput(t *testing.T) {
	f := newFixture(t)
	if err := os.Remove(f.config); err != nil {
		t.Fatal(err)
	}
	f.fake("kubectl", anyCall("unexpected", 99))
	for _, screen := range []string{"selector", "logs", "deploy", "analyze"} {
		out, diagnostic, status := f.run("", "preview", screen, "--width", "64", "--plain")
		if status != 0 || diagnostic != "" || !strings.Contains(out, "dots kube") || !strings.Contains(out, "dots-stage") || strings.Contains(out, "\x1b") || len(f.calls()) != 0 {
			t.Fatalf("%s %d %q %q", screen, status, out, diagnostic)
		}
	}
}

func TestMergedKubeconfigKeepsTheFileListForEveryRequest(t *testing.T) {
	f := newFixture(t)
	credentials := filepath.Join(f.home, "credentials.yaml")
	contexts := filepath.Join(f.home, "contexts.yaml")
	writeConfig(t, credentials, "apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster: {server: https://127.0.0.1:1}\nusers:\n- name: fixture-user\n  user: {token: MERGED-SECRET}\n")
	writeConfig(t, contexts, "apiVersion: v1\nkind: Config\ncurrent-context: another\ncontexts:\n- name: dots-stage\n  context: {cluster: fixture, user: fixture-user, namespace: stage-ns}\n")
	chain := credentials + string(os.PathListSeparator) + contexts
	f.set("KUBECONFIG", chain)
	f.fake("kubectl", exact([]string{"--context=dots-stage", "--namespace=stage-ns", "get", "pods"}, "pods\n", 0))
	out, diagnostic, status := f.run("", "kpstage", "get", "pods")
	if status != 0 || out != "pods\n" {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
	calls := f.calls()
	if len(calls) != 1 || calls[0].Kubeconfig != chain {
		t.Fatalf("chain %+v", calls)
	}
	for _, arg := range calls[0].Args {
		if strings.HasPrefix(arg, "--kubeconfig") {
			t.Fatal("merged chain passed as a single-file flag")
		}
	}
}
func TestUnconfiguredAliasNeverFallsBackToCurrentContext(t *testing.T) {
	f := newFixture(t)
	data, _ := os.ReadFile(f.config)
	writeConfig(t, f.config, strings.Replace(string(data), "name: dots-stage", "name: unbound-stage", 1))
	f.fake("kubectl", anyCall("unexpected", 99))
	out, diagnostic, status := f.run("", "kpstage", "get", "pods")
	if status == 0 || out != "" || len(f.calls()) != 0 || !strings.Contains(diagnostic, "setup") {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
}
func TestConflictingContextDefinitionsFailClosed(t *testing.T) {
	f := newFixture(t)
	second := filepath.Join(f.home, "second.yaml")
	writeConfig(t, second, "apiVersion: v1\nkind: Config\ncontexts:\n- name: dots-stage\n  context: {cluster: fixture, user: fixture-user, namespace: wrong}\n")
	f.set("KUBECONFIG", f.config+string(os.PathListSeparator)+second)
	f.fake("kubectl", anyCall("unexpected", 99))
	_, diagnostic, status := f.run("", "kpstage", "get", "pods")
	if status == 0 || len(f.calls()) != 0 || !strings.Contains(diagnostic, "conflicting") {
		t.Fatalf("%d %q", status, diagnostic)
	}
}
func TestSetupRequiresExplicitApprovalWithoutATerminal(t *testing.T) {
	f := newFixture(t)
	original, _ := os.ReadFile(f.config)
	_, diagnostic, status := f.run("yes\n", "setup", "stage", "--context", "unrelated", "--namespace", "changed", "--skip-aliases")
	data, _ := os.ReadFile(f.config)
	if status == 0 || string(data) != string(original) || len(f.calls()) != 0 || !strings.Contains(diagnostic, "--yes") {
		t.Fatalf("%d %q %+v", status, diagnostic, f.calls())
	}
}
func TestReadOnlySourceCanBeConfiguredWithoutReplacingIt(t *testing.T) {
	f := newFixture(t)
	original, _ := os.ReadFile(f.config)
	if err := os.Chmod(f.config, 0400); err != nil {
		t.Fatal(err)
	}
	_, diagnostic, status := f.run("", "setup", "stage", "--context", "unrelated", "--namespace", "changed", "--yes", "--skip-aliases")
	data, _ := os.ReadFile(f.config)
	if status != 0 || string(data) != string(original) {
		t.Fatalf("%d %q", status, diagnostic)
	}
	out, diagnostic, status := f.run("", "config", "--json")
	if status != 0 || !strings.Contains(out, `"namespace": "changed"`) {
		t.Fatalf("read-only source %d %q %q", status, out, diagnostic)
	}
}
func TestImportFailureDoesNotExposeRawCredentialsOrChangeFiles(t *testing.T) {
	f := newFixture(t)
	original, _ := os.ReadFile(f.config)
	incoming := filepath.Join(f.home, "copy.yaml")
	writeConfig(t, incoming, string(original))
	f.fake("kubectl", rule{Args: []string{"config", "view", "--flatten", "--raw", "-o", "yaml"}, Responses: []response{{Output: "RAW-SECRET", Stderr: "PRIVATE-DIAGNOSTIC", Code: 7}}})
	out, diagnostic, status := f.run("", "setup", "stage", "--import", incoming, "--context", "unrelated", "--yes", "--skip-aliases")
	data, _ := os.ReadFile(f.config)
	if status == 0 || string(data) != string(original) || strings.Contains(out+diagnostic, "RAW-SECRET") || strings.Contains(out+diagnostic, "PRIVATE-DIAGNOSTIC") {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
}
func TestDoctorChecksMetadataAndDependenciesWithoutNetwork(t *testing.T) {
	f := newFixture(t)
	out, diagnostic, status := f.run("", "doctor", "--json")
	if status != 1 || !strings.Contains(out, `"kubectl_available": false`) || diagnostic != "" {
		t.Fatalf("missing dependency %d %q %q", status, out, diagnostic)
	}
	f.fake("kubectl", anyCall("unexpected", 99))
	out, diagnostic, status = f.run("", "doctor", "--json", "--strict")
	if status != 0 || diagnostic != "" || len(f.calls()) != 0 {
		t.Fatalf("doctor %d %q %q", status, out, diagnostic)
	}
}

func TestNativeContextAliasDoesNotSwitchCurrentContext(t *testing.T) {
	f := newFixture(t)
	f.set("KUBECONFIG", "")
	f.set("KUBE_STAGE_KUBECONFIG", "")
	f.set("KUBE_STAGE_CONTEXT", "")
	path := filepath.Join(f.home, ".kube", "config")
	original := `apiVersion: v1
kind: Config
current-context: unrelated
clusters:
- name: fixture
  cluster:
    server: https://127.0.0.1:1
users:
- name: fixture-user
  user:
    token: DO-NOT-PRINT-SECRET
contexts:
- name: unrelated
  context: {cluster: fixture, user: fixture-user, namespace: wrong}
- name: dots-stage
  context: {cluster: fixture, user: fixture-user, namespace: app-stage}
`
	writeConfig(t, path, original)
	f.fake("kubectl", exact([]string{"--context=dots-stage", "--namespace=app-stage", "get", "pods"}, "pods\n", 0))
	f.fake("zsh", anyCall("unexpected", 99))
	out, diagnostic, status := f.run("", "kpstage", "get", "pods")
	if status != 0 || out != "pods\n" || !strings.Contains(diagnostic, "context=dots-stage") {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("alias changed kubeconfig/current-context")
	}
	calls := f.calls()
	if len(calls) != 1 || calls[0].Kubeconfig != path {
		t.Fatalf("source %+v", calls)
	}
}
