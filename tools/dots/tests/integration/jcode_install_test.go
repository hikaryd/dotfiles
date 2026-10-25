package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dots/native/internal/setup"
)

// No network, containers or package execution: verify original-tool ordering.
func TestJcodeInstallCompletePackageAndSelectedNode(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	put(t, filepath.Join(root, "scripts/jcode-install.sh"), get(t, filepath.Join(repo, "scripts/jcode-install.sh")))
	for _, name := range []string{"package.json", "package-lock.json", "web-search.example.json", "python-tools.txt"} {
		put(t, filepath.Join(root, "config/jcode", name), get(t, filepath.Join(repo, "config/jcode", name)))
	}
	put(t, filepath.Join(root, "config/jcode/neo4j.compose.yml"), "services: {}\n")
	put(t, filepath.Join(root, "scripts/build-go-tools.sh"), "#!/bin/sh\nexit 0\n")
	put(t, filepath.Join(root, "build/bin/dots-tools"), `#!/bin/sh
printf '%s\n' "$*" >>"$CALL_LOG"
if [ "$2" = jcode-neo4j-init ]; then
 mkdir -p "$JCODE_INTEGRATIONS_DIR/neo4j"
 printf 'NEO4J_PASSWORD=%064d\n' 1 >"$JCODE_INTEGRATIONS_DIR/neo4j/private.env"
fi
`)
	bin := filepath.Join(root, "bin")
	node := filepath.Join(bin, "selected node")
	put(t, node, "#!/bin/sh\nprintf 'cwd=%s\\nargs=%s\\n' \"$PWD\" \"$*\" >>\"$CALL_LOG\"\n")
	put(t, filepath.Join(bin, "node"), "#!/bin/sh\nexit 97\n")
	put(t, filepath.Join(bin, "npm"), "#!/bin/sh\nexit 0\n")
	put(t, filepath.Join(bin, "python"), "#!/bin/sh\nprintf 'ready\\n' >>\"$CALL_LOG\"\ncat >/dev/null\n")
	put(t, filepath.Join(bin, "serena"), "#!/bin/sh\nprintf 'health-check\\n' >>\"$CALL_LOG\"\n")
	put(t, filepath.Join(bin, "docker"), "#!/bin/sh\nprintf 'docker %s\\n' \"$*\" >>\"$CALL_LOG\"\n")
	put(t, filepath.Join(bin, "uv"), `#!/bin/sh
printf 'uv %s\n' "$*" >>"$CALL_LOG"
case "$1 $2" in
 'python find') printf '%s/bin/python\n' "$FIXTURE";;
 venv*) for arg do last=$arg; done; mkdir -p "$last/bin"; cp "$FIXTURE/bin/python" "$FIXTURE/bin/serena" "$last/bin/";;
esac
`)
	for _, p := range []string{node, filepath.Join(bin, "node"), filepath.Join(bin, "npm"), filepath.Join(bin, "python"), filepath.Join(bin, "serena"), filepath.Join(bin, "docker"), filepath.Join(bin, "uv"), filepath.Join(root, "scripts/build-go-tools.sh"), filepath.Join(root, "build/bin/dots-tools")} {
		if err := os.Chmod(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	data := filepath.Join(root, "empty data")
	log := filepath.Join(root, "calls.log")
	cmd := exec.Command("bash", filepath.Join(root, "scripts/jcode-install.sh"))
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "JCODE_NODE_BIN="+node, "JCODE_INTEGRATIONS_DIR="+data, "CALL_LOG="+log, "FIXTURE="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for _, rel := range []string{"runtime/package.json", "runtime/package-lock.json", "web/config/web-search.json"} {
		if _, err := os.Stat(filepath.Join(data, rel)); err != nil {
			t.Fatal(rel, err)
		}
	}
	calls := get(t, log)
	if !strings.Contains(calls, "cwd="+filepath.Join(data, "runtime")+"\n") || !strings.Contains(calls, "ci --ignore-scripts") {
		t.Fatal(calls)
	}
	health, ready, apply := strings.Index(calls, "health-check"), strings.Index(calls, "ready\n"), strings.Index(calls, "setup jcode-apply")
	if health < 0 || ready < health || apply < ready || !strings.Contains(calls, "docker compose") || !strings.Contains(calls, "--wait") {
		t.Fatal("must smoke Serena and wait for Neo4j BEFORE publishing live MCP", calls)
	}
	if strings.Contains(calls, ".mjs") {
		t.Fatal("must not execute our JS glue", calls)
	}
}

func TestJcodeUpdateCheckNeverInstalls(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	put(t, filepath.Join(root, "scripts/jcode-update.sh"), get(t, filepath.Join(repo, "scripts/jcode-update.sh")))
	put(t, filepath.Join(root, "scripts/jcode-install.sh"), "#!/bin/sh\nprintf 'install\\n' >>\"$CALL_LOG\"\n")
	put(t, filepath.Join(root, "config/jcode/python-tools.txt"), "serena-agent==2.1.0\n")
	bin := filepath.Join(root, "bin")
	for _, name := range []string{"node", "docker", "python"} {
		p := filepath.Join(bin, name)
		put(t, p, "#!/bin/sh\nprintf 'check\\n' >>\"$CALL_LOG\"\n")
		if err := os.Chmod(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	data := filepath.Join(root, "data")
	put(t, filepath.Join(data, "python/serena-agent/bin/python"), get(t, filepath.Join(bin, "python")))
	if err := os.Chmod(filepath.Join(data, "python/serena-agent/bin/python"), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "calls.log")
	invoke := func(args ...string) error {
		cmd := exec.Command("bash", append([]string{filepath.Join(root, "scripts/jcode-update.sh")}, args...)...)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALL_LOG="+log, "JCODE_INTEGRATIONS_DIR="+data)
		_, err := cmd.CombinedOutput()
		return err
	}
	if err := invoke("--check"); err != nil || strings.Contains(get(t, log), "install") {
		t.Fatal("check must only inspect versions/status, never install", err)
	}
	if err := invoke("--unknown"); err == nil || strings.Contains(get(t, log), "install") {
		t.Fatal("unknown option must fail without mutation")
	}
	if err := invoke(); err != nil || !strings.HasSuffix(get(t, log), "install\n") {
		t.Fatal("default update must reinstall pinned setup", err)
	}
}

func TestJcodeNeo4jInitDoesNotPublishConfig(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	config := filepath.Join(root, "config")
	t.Setenv("JCODE_INTEGRATIONS_DIR", data)
	t.Setenv("JCODE_CONFIG_DIR", config)
	if code := setup.Run(root, []string{"jcode-neo4j-init"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(data, "neo4j/private.env")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("init must not publish native MCP", err)
	}
}
