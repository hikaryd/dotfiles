package integration_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The actual Nushell bridge is the interface; only the external navigation
// command is substituted. PTY rendering is covered separately by shell tests.
func TestNushellBridgePreservesNavigationStatusAndCwd(t *testing.T) {
	nu, err := exec.LookPath("nu")
	if err != nil {
		t.Skip("Nushell not installed")
	}
	bridge, err := filepath.Abs("../../../../config/nushell/bridge.nu")
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(bridge)
	for _, status := range []string{"0", "7"} {
		t.Run(status, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "directory with spaces 界")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "scripts"), 0700); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\nprintf '%s' \"$TEST_DIRECTORY\" > \"$2\"\nexit \"$TEST_EXIT_CODE\"\n"
			if err := os.WriteFile(filepath.Join(root, "scripts/dots-zsh"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(nu, "--no-config-file", "--commands", "source "+string(quoted)+"; try { dots-cd fixture } catch {}; print $\"EXIT=($env.LAST_EXIT_CODE)\"; print $\"CWD=($env.PWD)\"")
			command.Env = []string{"HOME=" + root, "PATH=/usr/bin:/bin", "DOTS_ROOT=" + root, "TEST_DIRECTORY=" + directory, "TEST_EXIT_CODE=" + status, "LC_ALL=en_US.UTF-8", "NO_COLOR=1"}
			command.Dir = root
			output, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "EXIT="+status) || !strings.Contains(string(output), "CWD="+directory) {
				t.Fatalf("navigation status=%s: %v %q", status, err, output)
			}
		})
	}
}
