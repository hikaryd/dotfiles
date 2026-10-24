package kube

import (
	"strings"
	"testing"
)

func TestPipedSingleLogsStayRawEvenWithForcedColor(t *testing.T) {
	f := newFixture(t)
	f.set("NO_COLOR", "")
	f.set("KUBE_COLOR", "always")
	raw := "ERROR application record\n"
	f.fake("kubectl", exact(pinned("stage", "get", "pods", "-o", "json"), `{"items":[{"metadata":{"name":"api"}}]}`, 0), exact(pinned("stage", "logs", "-f", "pod/api", "--all-containers=true", "--prefix=true", "--tail=200"), raw, 0))
	out, diagnostic, status := f.run("", "kpstage-logs", "api")
	if status != 0 || out != raw {
		t.Fatalf("pipe was decorated: %d %q %q", status, out, diagnostic)
	}
}

func TestPipedMultiLogsPreserveRawOutputDiagnosticsAndExitCode(t *testing.T) {
	for _, status := range []int{0, 17} {
		f := newFixture(t)
		raw := "[pod/api/app] INFO raw\tmessage\n"
		diagnostic := "Warning: fixture diagnostic\n"
		helper := []string{"--context", "dots-stage", "--namespace", "stage-namespace", "logs", "-f", "pod/api", "--all-containers=true", "--prefix=true", "--tail=200"}
		f.fake("kubectl", exact(pinned("stage", "get", "pods", "-o", "json"), `{"items":[{"metadata":{"name":"api"}}]}`, 0), rule{Args: helper, Responses: []response{{Output: raw, Stderr: diagnostic, Code: status}}})
		out, stderr, code := f.run("", "kpstage-logs-multi", "api")
		if code != status || out != raw || !strings.Contains(stderr, diagnostic) {
			t.Fatalf("plain multi logs lost contract: %d/%d out=%q stderr=%q", code, status, out, stderr)
		}
	}
}

func TestPipedMultiLogFilterPreservesMatchingBytesAndFinalRecord(t *testing.T) {
	f := newFixture(t)
	f.set("KUBE_MULTI_LOG_FILTER", "1")
	raw := "[pod/api/app] needle\tvalue\n[ pod ] skipped\n[ pod ] needle final"
	helper := []string{"--context", "dots-stage", "--namespace", "stage-namespace", "logs", "-f", "pod/api", "--all-containers=true", "--prefix=true", "--tail=200"}
	f.fake("kubectl", exact(pinned("stage", "get", "pods", "-o", "json"), `{"items":[{"metadata":{"name":"api"}}]}`, 0), exact(helper, raw, 0))
	out, diagnostic, status := f.run("", "kpstage-logs-multi", "api", "needle")
	if status != 0 || out != "[pod/api/app] needle\tvalue\n[ pod ] needle final" {
		t.Fatalf("filter altered matching bytes: %d %q %q", status, out, diagnostic)
	}
}

func TestCronRunAcceptsActiveJobReferences(t *testing.T) {
	f := newFixture(t)
	create := exact(pinned("preprod", "create", "job", "--from=cronjob/nightly"), "created\n", 0)
	create.Prefix = true
	logs := exact(pinned("preprod", "logs", "-f"), "job complete\n", 0)
	logs.Prefix = true
	f.fake("kubectl", exact(pinned("preprod", "get", "cronjobs", "-o", "json"), `{"items":[{"kind":"CronJob","metadata":{"name":"nightly"},"status":{"active":[{"kind":"Job","name":"nightly-scheduled","namespace":"preprod-namespace","uid":"active-job"}],"lastScheduleTime":"2026-01-01T00:00:00Z"}}]}`, 0), create, logs)
	out, diagnostic, status := f.run("", "kppreprod-cron-run", "nightly")
	if status != 0 || !strings.Contains(out, "job complete") {
		t.Fatalf("CronJob active references broke selector: %d %q %q", status, out, diagnostic)
	}
}

func TestPodAnalyticsDoesNotDecodeKubectlWarningsAsJSON(t *testing.T) {
	f := newFixture(t)
	f.set("KUBE_POD_ANALYZE_ONCE", "1")
	f.set("KUBE_POD_ANALYZE_PROCESSES", "false")
	pod := `{"kind":"Pod","metadata":{"name":"api","uid":"fixture"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Running"}}`
	helper := []string{"--context", "dots-stage", "--namespace", "stage-namespace"}
	f.fake("kubectl", exact(pinned("stage", "get", "pods", "-o", "json"), `{"items":[`+pod+`]}`, 0), rule{Args: append(append([]string{}, helper...), "get", "pod/api", "-o", "json"), Responses: []response{{Output: pod, Stderr: "Warning: API deprecation\n"}}}, exact(append(append([]string{}, helper...), "top", "pod", "api", "--containers", "--no-headers"), "api app 10m 32Mi\n", 0), rule{Args: append(append([]string{}, helper...), "get", "events"), Prefix: true, Responses: []response{{Output: `{"items":[]}`, Stderr: "Warning: API deprecation\n"}}})
	out, diagnostic, status := f.run("", "kpstage-pod-analyze", "api")
	if status != 0 || !strings.Contains(out, "api") {
		t.Fatalf("successful response rejected because of warning: %d %q %q", status, out, diagnostic)
	}
}
