package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentAliasesShowLocalConfigurationWithoutKubectl(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", anyCall("unexpected", 99))
	for _, alias := range []string{"kpstage", "kpprod"} {
		out, diagnostic, status := f.run("", alias, "config", "--json")
		if status != 0 || diagnostic != "" || !strings.Contains(out, `"context": "dots-`+strings.TrimPrefix(alias, "kp")+`"`) || strings.Count(out, `"environment":`) != 1 || len(f.calls()) != 0 {
			t.Fatalf("%s %d %q %q", alias, status, out, diagnostic)
		}
	}
}

func TestEnvironmentsCanUseSeparateFilesWithDuplicateNames(t *testing.T) {
	f := newFixture(t)
	stageDoc, _ := os.ReadFile(f.config)
	stage := filepath.Join(f.home, "stage.yaml")
	prod := filepath.Join(f.home, "prod.yaml")
	prodDoc := strings.ReplaceAll(string(stageDoc), "https://127.0.0.1:1", "https://127.0.0.1:2")
	prodDoc = strings.ReplaceAll(prodDoc, "fixture-secret", "OTHER-SECRET")
	writeConfig(t, stage, string(stageDoc))
	writeConfig(t, prod, prodDoc)
	for _, row := range []struct{ env, path, namespace string }{{"stage", stage, "stage-app"}, {"prod", prod, "prod-app"}} {
		out, diagnostic, status := f.run("", "setup", row.env, "--kubeconfig", row.path, "--context", "unrelated", "--namespace", row.namespace, "--yes", "--skip-aliases")
		if status != 0 {
			t.Fatalf("%s setup %d %q %q", row.env, status, out, diagnostic)
		}
	}
	first, _ := os.ReadFile(stage)
	second, _ := os.ReadFile(prod)
	if string(first) != string(stageDoc) || string(second) != prodDoc {
		t.Fatal("setup changed administrator-provided files")
	}
	f.fake("kubectl", exact([]string{"--context=dots-stage", "--namespace=stage-app", "get", "pods"}, "stage pods\n", 0), exact([]string{"--context=dots-prod", "--namespace=prod-app", "get", "pods"}, "prod pods\n", 0))
	for _, row := range []struct{ alias, want string }{{"kpstage", "stage pods\n"}, {"kpprod", "prod pods\n"}} {
		out, diagnostic, status := f.run("", row.alias, "get", "pods")
		if status != 0 || out != row.want {
			t.Fatalf("%s %d %q %q", row.alias, status, out, diagnostic)
		}
	}
	calls := f.calls()
	if len(calls) != 2 || !strings.Contains(calls[0].Kubeconfig, stage) || strings.Contains(calls[0].Kubeconfig, prod) || !strings.Contains(calls[1].Kubeconfig, prod) || strings.Contains(calls[1].Kubeconfig, stage) {
		t.Fatalf("mixed environments %+v", calls)
	}
	out, diagnostic, status := f.run("", "config", "--json")
	if status != 0 || !strings.Contains(out, `"namespace": "stage-app"`) || !strings.Contains(out, `"namespace": "prod-app"`) || strings.Contains(out, "OTHER-SECRET") {
		t.Fatalf("config %d %q %q", status, out, diagnostic)
	}
}

func TestSetupRemembersSourcesWithoutShellExports(t *testing.T) {
	f := newFixture(t)
	document, err := os.ReadFile(f.config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.config); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(f.config, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(f.config, "provided.yaml")
	writeConfig(t, source, string(document))
	f.fake("kubectl", exact([]string{"--context=dots-stage", "--namespace=chosen-ns", "get", "pods"}, "pods\n", 0))
	out, diagnostic, status := f.run("", "setup", "stage", "--kubeconfig", source, "--context", "unrelated", "--namespace", "chosen-ns", "--yes", "--skip-aliases")
	if status != 0 || strings.Contains(out+diagnostic, "add KUBECONFIG=") {
		t.Fatalf("setup %d %q %q", status, out, diagnostic)
	}
	out, diagnostic, status = f.run("", "config", "--json")
	if status != 0 || diagnostic != "" || !strings.Contains(out, source) || !strings.Contains(out, `"namespace": "chosen-ns"`) || strings.Contains(out, "fixture-secret") {
		t.Fatalf("remembered source %d %q %q", status, out, diagnostic)
	}
	out, diagnostic, status = f.run("", "kpstage", "get", "pods")
	if status != 0 || out != "pods\n" {
		t.Fatalf("alias %d %q %q", status, out, diagnostic)
	}
	if len(f.calls()) != 1 || !strings.HasSuffix(f.calls()[0].Kubeconfig, string(os.PathListSeparator)+source) {
		t.Fatalf("wrong persisted source %+v", f.calls())
	}
}
