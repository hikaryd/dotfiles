package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalAliasFlagsReachLocalParser(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", anyCall("unexpected", 99))
	for _, args := range [][]string{{"kpstage", "config", "--kubeconfig", f.config, "--json"}, {"kpstage", "setup", "--context", "unrelated", "--namespace", "chosen", "--yes", "--skip-aliases"}} {
		out, diag, code := f.run("", args...)
		if code != 0 {
			t.Fatalf("%v: %d %s %s", args, code, out, diag)
		}
	}
	if len(f.calls()) != 0 {
		t.Fatal("local setup contacted cluster")
	}
}
func TestSaveConfigsRejectsDuplicateDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	a := &configFile{path: path}
	b := &configFile{path: path}
	rootNode(a)
	rootNode(b)
	if err := saveConfigs([]*configFile{a, b}, map[*configFile]bool{a: true, b: true}); err == nil {
		t.Fatal("duplicate destination accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("duplicate transaction changed file")
	}
}
func TestSaveConfigsRejectsDirectoryAliases(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			dir := t.TempDir()
			real := filepath.Join(dir, "real")
			alias := filepath.Join(dir, "alias")
			if err := os.Mkdir(real, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, alias); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(real, "nested", "config")
			data := []byte("apiVersion: v1\nkind: Config\n")
			if existing {
				writeConfig(t, path, string(data))
			}
			a := &configFile{path: path, exists: existing}
			b := &configFile{path: filepath.Join(alias, "nested", "config"), exists: existing}
			if existing {
				a.data, b.data = data, data
			}
			rootNode(a)
			rootNode(b)
			if err := saveConfigs([]*configFile{a, b}, map[*configFile]bool{a: true, b: true}); err == nil {
				t.Fatal("directory aliases accepted as separate destinations")
			}
			got, err := os.ReadFile(path)
			if existing && (err != nil || string(got) != string(data)) {
				t.Fatalf("existing config changed: %s %v", got, err)
			}
			if !existing && !os.IsNotExist(err) {
				t.Fatal("duplicate transaction created config")
			}
		})
	}
}
func TestImportDoesNotUseGeneratedOverlay(t *testing.T) {
	for _, aliased := range []bool{false, true} {
		t.Run(fmt.Sprint(aliased), func(t *testing.T) {
			f := newFixture(t)
			// Install a generated overlay, then resolve the environment's remembered chain.
			s := &session{ctx: context.Background(), env: f.env, target: target{environment: "stage"}}
			overlay := s.contextFilePath("stage")
			writeConfig(t, overlay, "apiVersion: v1\nkind: Config\ncontexts: []\n")
			source := overlay
			if aliased {
				alias := filepath.Join(f.home, "overlay-alias")
				if err := os.Symlink(filepath.Dir(overlay), alias); err != nil {
					t.Fatal(err)
				}
				source = filepath.Join(alias, filepath.Base(overlay))
			}
			s.useSources([]string{source, f.config})
			base, err := s.inventory()
			if err != nil {
				t.Fatal(err)
			}
			document, _ := os.ReadFile(f.config)
			incoming := filepath.Join(f.home, "incoming.yaml")
			// Unique entries, avoiding first-wins credential conflicts.
			writeConfig(t, incoming, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(document), "fixture", "incoming"), "dots-", "incoming-"), "unrelated", "incoming-context"))
			config, err := s.importInventory(base, []string{incoming})
			if err != nil {
				t.Fatal(err)
			}
			f.fake("kubectl", exact([]string{"config", "view", "--flatten", "--raw", "-o", "yaml"}, string(document), 0))
			// exec.LookPath uses the parent's PATH, whereas the helper gets s.env.
			t.Setenv("PATH", f.bin)
			imported, err := s.prepareImport(base, config, nil)
			if err != nil {
				t.Fatal(err)
			}
			if imported.path != f.config {
				t.Fatalf("flattened import targets generated overlay: %s", imported.path)
			}
		})
	}
}

func TestRestartWaitsForControllerSpecificReplacement(t *testing.T) {
	for _, kind := range []string{"StatefulSet", "DaemonSet"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.set("KUBE_POD_RESTART_FOLLOW_LOGS", "0")
			f.set("KUBE_POD_RESTART_POLL_INTERVAL", "1")
			owner := fmt.Sprintf("{\"kind\":%q,\"name\":\"owner\",\"uid\":\"controller\",\"controller\":true}", kind)
			pod := func(name, uid, node, ready string) string {
				return fmt.Sprintf("{\"metadata\":{\"name\":%q,\"uid\":%q,\"ownerReferences\":[%s]},\"spec\":{\"nodeName\":%q},\"status\":{\"phase\":\"Running\",\"conditions\":[{\"type\":\"Ready\",\"status\":%q}]}}", name, uid, owner, node, ready)
			}
			old := pod("api-0", "old", "node-a", "True")
			other := pod("api-1", "other", "node-b", "True")
			replacementName := "api-0"
			if kind == "DaemonSet" {
				replacementName = "api-new"
			}
			pending := pod(replacementName, "new", "node-a", "False")
			ready := pod(replacementName, "new", "node-a", "True")
			f.fake("kubectl", exact(pinned("dev", "get", "pods", "-o", "json"), "{\"items\":["+old+"]}", 0),
				exact(pinned("dev", "get", "pod/api-0", "-o", "json"), old, 0),
				exact(pinned("dev", "get", strings.ToLower(kind)+"/owner", "-o", "json"), "{\"metadata\":{\"uid\":\"controller\"},\"spec\":{\"selector\":{\"matchLabels\":{\"app\":\"api\"}}}}", 0),
				rule{Args: pinned("dev", "get", "pods", "-o", "json", "-l", "app=api"), Responses: []response{{Output: "{\"items\":[" + old + "]}"}, {Output: "{\"items\":[" + other + "," + pending + "]}"}, {Output: "{\"items\":[" + ready + "]}"}}},
				exact(pinned("dev", "delete", "pod/api-0", "--wait=false"), "deleted\n", 0))
			out, diag, code := f.run("", "kpdev-pod-restart", "api-0")
			if code != 0 || !strings.Contains(out, "pod/"+replacementName+" is Ready") {
				t.Fatalf("%d %s %s", code, out, diag)
			}
		})
	}
}
