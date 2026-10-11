package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryKubeconfigGivesFileRecoveryInstructionsWithoutTouchingIt(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "default"
		if explicit {
			name = "exported"
		}
		t.Run(name, func(t *testing.T) {
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
			supplied := filepath.Join(f.config, "provided.yaml")
			writeConfig(t, supplied, string(document))
			if !explicit {
				f.set("KUBECONFIG", "")
			}
			f.fake("kubectl", anyCall("unexpected", 99))
			for _, args := range [][]string{{"config"}, {"config", "--json"}, {"setup", "stage", "--context", "unrelated", "--yes", "--skip-aliases"}, {"kpstage", "get", "pods"}} {
				out, diagnostic, status := f.run("", args...)
				if status == 0 || out != "" || !strings.Contains(diagnostic, "directory") || !strings.Contains(diagnostic, "KUBECONFIG") || !strings.Contains(diagnostic, "setup --kubeconfig") {
					t.Fatalf("%v: %d %q %q", args, status, out, diagnostic)
				}
			}
			info, err := os.Stat(f.config)
			if err != nil || !info.IsDir() {
				t.Fatal("directory was replaced")
			}
			contents, _ := os.ReadFile(supplied)
			if string(contents) != string(document) || len(f.calls()) != 0 {
				t.Fatal("kubeconfig data was changed or kubectl was called")
			}
			out, diagnostic, status := f.run("", "config", "--kubeconfig", supplied, "--json")
			if status != 0 || diagnostic != "" || !strings.Contains(out, `"current_context": "unrelated"`) {
				t.Fatalf("explicit recovery %d %q %q", status, out, diagnostic)
			}
		})
	}
}
