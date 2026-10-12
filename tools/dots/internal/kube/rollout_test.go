package kube

import (
	"strings"
	"testing"
)

func TestRolloutRestartRejectsUnexpectedArgumentsBeforeClusterCalls(t *testing.T) {
	for _, args := range [][]string{{"--context=foreign"}, {"api", "--namespace=foreign"}, {"--all-namespaces"}, {"--wait"}, {"api", "extra"}} {
		f := newFixture(t)
		f.fake("kubectl", anyCall(`{"items":[]}`, 0))
		_, diagnostic, status := f.run("", append([]string{"kpstage-rollout-restart"}, args...)...)
		if status != 2 || len(f.calls()) != 0 {
			t.Fatalf("invalid args %v: %d %q calls=%+v", args, status, diagnostic, f.calls())
		}
	}
}

func TestRolloutRestartRefusesStrategiesThatWouldNotRestartPods(t *testing.T) {
	for _, workload := range []string{
		`{"kind":"Deployment","metadata":{"name":"api"},"spec":{"paused":true}}`,
		`{"kind":"StatefulSet","metadata":{"name":"api"},"spec":{"updateStrategy":{"type":"OnDelete"}}}`,
		`{"kind":"DaemonSet","metadata":{"name":"api"},"spec":{"updateStrategy":{"type":"OnDelete"}}}`,
	} {
		f := newFixture(t)
		f.fake("kubectl", exact(pinned("stage", "get", "deployments,statefulsets,daemonsets", "-o", "json"), `{"items":[{"kind":"Deployment","metadata":{"name":"api-healthy"}},`+workload+`]}`, 0))
		out, diagnostic, status := f.run("", "kpstage-rollout-restart", "api")
		if status == 0 || !strings.Contains(out+diagnostic, "pod-restart") {
			t.Fatalf("non-rolling workload silently accepted: %d %q %q", status, out, diagnostic)
		}
	}
}

func TestRolloutRestartPreservesProdGuardAndMutationFailure(t *testing.T) {
	t.Run("prod requires confirmation", func(t *testing.T) {
		f := newFixture(t)
		f.fake("kubectl", anyCall("", 0))
		_, diagnostic, status := f.run("yes\n", "kpprod-rollout-restart", "api")
		if status == 0 || len(f.calls()) != 0 || !strings.Contains(diagnostic, "MUTATING action=rollout-restart") {
			t.Fatalf("prod guard %d %q %+v", status, diagnostic, f.calls())
		}
	})
	t.Run("confirmed prod reports failure", func(t *testing.T) {
		f := newFixture(t)
		f.set("KPROD_CONFIRMED", "1")
		f.fake("kubectl", exact(pinned("prod", "get", "deployments,statefulsets,daemonsets", "-o", "json"), `{"items":[{"kind":"DaemonSet","metadata":{"name":"api-agent"}}]}`, 0), rule{Args: pinned("prod", "rollout", "restart", "daemonset/api-agent"), Responses: []response{{Stderr: "restart denied\n", Code: 19}}})
		out, diagnostic, status := f.run("", "kpprod-rollout-restart", "api-agent")
		if status != 19 || !strings.Contains(diagnostic, "restart denied") || strings.Contains(out, "Restart requested;") {
			t.Fatalf("mutation failure %d %q %q", status, out, diagnostic)
		}
	})
	t.Run("restart aliases advertised for all environments", func(t *testing.T) {
		f := newFixture(t)
		out, diagnostic, status := f.run("", "commands")
		if status != 0 {
			t.Fatalf("commands %d %q", status, diagnostic)
		}
		for _, name := range []string{"kpdev-rollout-restart", "kpstage-rollout-restart", "kppreprod-rollout-restart", "kpprod-rollout-restart"} {
			if !strings.Contains(out, name+"\n") {
				t.Fatalf("missing command %s", name)
			}
		}
	})
}

func TestRolloutRestartStartsSelectedWorkloadsWithoutWaitingForPods(t *testing.T) {
	f := newFixture(t)
	workloads := `{"items":[{"kind":"StatefulSet","metadata":{"name":"api-worker"}},{"kind":"Deployment","metadata":{"name":"api"}},{"kind":"DaemonSet","metadata":{"name":"agent"}}]}`
	f.fake("kubectl", exact(pinned("stage", "get", "deployments,statefulsets,daemonsets", "-o", "json"), workloads, 0), exact(pinned("stage", "rollout", "restart", "deployment/api", "statefulset/api-worker"), "deployment.apps/api restarted\nstatefulset.apps/api-worker restarted\n", 0))
	out, diagnostic, status := f.run("", "kpstage-rollout-restart", "api")
	if status != 0 || !strings.Contains(out, "deployment.apps/api restarted") || !strings.Contains(out, "statefulset.apps/api-worker restarted") {
		t.Fatalf("restart %d %q %q", status, out, diagnostic)
	}
	for _, call := range f.calls() {
		if call.Kubeconfig != f.config {
			t.Fatalf("unpinned restart source: %+v", call)
		}
	}
}
