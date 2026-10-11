package kube

import (
	"os"
	"strings"
	"testing"
)

func TestNamespaceListUsesOnlyThePinnedClusterAndDoesNotWrite(t *testing.T) {
	f := newFixture(t)
	original, _ := os.ReadFile(f.config)
	f.fake("kubectl", exact([]string{"--context=dots-stage", "get", "namespaces", "-o", "json", "--request-timeout=10s"}, `{"items":[{"metadata":{"name":"default"}},{"metadata":{"name":"application"}}]}`, 0))
	out, diagnostic, status := f.run("", "kpstage", "namespace", "--list", "--json")
	if status != 0 || diagnostic != "" || !strings.Contains(out, `"application"`) || !strings.Contains(out, `"default"`) {
		t.Fatalf("namespace list %d %q %q", status, out, diagnostic)
	}
	data, _ := os.ReadFile(f.config)
	if string(data) != string(original) || len(f.calls()) != 1 {
		t.Fatal("listing changed settings or queried another cluster")
	}
	if f.calls()[0].Kubeconfig != f.config {
		t.Fatal("namespace list changed the source")
	}
}

func TestNamespaceDiscoveryFailureIsReadOnlyAndDoesNotExposeDiagnostics(t *testing.T) {
	f := newFixture(t)
	original, _ := os.ReadFile(f.config)
	f.fake("kubectl", rule{Args: []string{"--context=dots-stage", "get", "namespaces", "-o", "json", "--request-timeout=10s"}, Responses: []response{{Stderr: "Forbidden PRIVATE-PLUGIN-DIAGNOSTIC", Code: 1}}})
	out, diagnostic, status := f.run("", "kpstage", "namespace", "--list", "--json")
	if status == 0 || out != "" || !strings.Contains(diagnostic, "list namespaces") || strings.Contains(diagnostic, "PRIVATE-PLUGIN-DIAGNOSTIC") || len(f.calls()) != 1 {
		t.Fatalf("failed discovery %d %q %q", status, out, diagnostic)
	}
	data, _ := os.ReadFile(f.config)
	if string(data) != string(original) {
		t.Fatal("failed discovery changed kubeconfig")
	}
}

func TestSetupDoesNotInventANamespaceAndEmptyLogsExplainRecovery(t *testing.T) {
	f := newFixture(t)
	document, _ := os.ReadFile(f.config)
	writeConfig(t, f.config, strings.Replace(string(document), "namespace: wrong", "namespace: ''", 1))
	_, diagnostic, status := f.run("", "setup", "stage", "--context", "unrelated", "--yes", "--skip-aliases")
	if status == 0 || !strings.Contains(diagnostic, "--namespace") {
		t.Fatalf("invented namespace %d %q", status, diagnostic)
	}
	f.fake("kubectl", exact([]string{"--context=dots-stage", "--namespace=stage-namespace", "get", "pods", "-o", "json"}, `{"items":[]}`, 0))
	_, diagnostic, status = f.run("", "kpstage-logs")
	if status == 0 || !strings.Contains(diagnostic, "namespace") || !strings.Contains(diagnostic, "kpstage setup") {
		t.Fatalf("empty pods %d %q", status, diagnostic)
	}
	if len(f.calls()) != 1 {
		t.Fatal("empty result probed a different namespace or cluster")
	}
}
