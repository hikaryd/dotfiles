package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneRolloutRestartPickerAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		name := "selected workloads"
		if cancel {
			name = "escape does not mutate"
		}
		t.Run(name, func(t *testing.T) {
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
 *' get deployments,statefulsets,daemonsets '*) printf '{"items":[{"kind":"Deployment","metadata":{"name":"api"},"spec":{"strategy":{"type":"RollingUpdate"}}},{"kind":"StatefulSet","metadata":{"name":"worker"},"spec":{"updateStrategy":{"type":"RollingUpdate"}}},{"kind":"DaemonSet","metadata":{"name":"agent"}}]}\n' ;;
 *' rollout restart deployment/api statefulset/worker '*) printf 'ROLLOUT_REQUESTED\n' ;;
 *) exit 97 ;;
esac
`
			if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(bin, "kpstage-rollout-restart")
			if err := os.Symlink(testBinary, alias); err != nil {
				t.Fatal(err)
			}
			terminal, capture, command := startUITestBinary(t, env, alias)
			awaitScreen(t, capture, "rollout restart workloads")
			awaitScreen(t, capture, "RollingUpdate")
			if cancel {
				terminal.Write([]byte{27})
				if err := command.Wait(); err == nil {
					t.Fatal("cancel unexpectedly succeeded")
				}
			} else {
				uiStep(t, terminal, capture, "deployment/api ", "selected")
				uiStep(t, terminal, capture, "\x0cstatefulset/worker ", "2")
				terminal.Write([]byte("\r"))
				if err := command.Wait(); err != nil {
					t.Fatalf("restart: %v screen=%q", err, capture.String())
				}
				if !strings.Contains(capture.String(), "ROLLOUT_REQUESTED") {
					t.Fatalf("no restart output: %q", capture.String())
				}
			}
			data, _ := os.ReadFile(calls)
			if cancel && strings.Contains(string(data), "rollout restart") {
				t.Fatalf("escape mutated cluster: %q", data)
			}
			if !cancel && (!strings.Contains(string(data), "rollout restart deployment/api statefulset/worker") || strings.Contains(string(data), "daemonset/agent") || strings.Contains(string(data), " get pods ")) {
				t.Fatalf("wrong rollout requests: %q", data)
			}
		})
	}
}
