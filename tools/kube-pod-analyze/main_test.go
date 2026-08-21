package main

import (
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultRefreshInterval(t *testing.T) {
	if defaultRefresh != 100*time.Millisecond {
		t.Fatalf("default refresh = %s, want 100ms", defaultRefresh)
	}
}

func TestExplicitLiveModeOverridesMissingTTYDetection(t *testing.T) {
	if !shouldRunLive(config{live: true}, false) {
		t.Fatal("explicit live mode must not depend on stdout TTY detection")
	}
	if shouldRunLive(config{once: true, live: true}, true) {
		t.Fatal("once mode must override live mode")
	}
}

func TestStringListAcceptsMultiplePodsAndDeduplicates(t *testing.T) {
	var pods stringList
	for _, pod := range []string{"example-api-a", "example-api-b", "example-api-a"} {
		if err := pods.Set(pod); err != nil {
			t.Fatal(err)
		}
	}
	if got := pods.String(); got != "example-api-a,example-api-b" {
		t.Fatalf("unexpected pods: %q", got)
	}
}

func TestParseProcSnapshotAndCalculateProcessUsage(t *testing.T) {
	previous, err := parseProcSnapshot("@\t10000\t8\n1\t0\t15\t1024\tinit\n42\t1\t850\t4096\tworker\n")
	if err != nil {
		t.Fatal(err)
	}
	current, err := parseProcSnapshot("@\t10400\t8\n1\t0\t17\t1024\tinit\n42\t1\t950\t4096\tworker\n")
	if err != nil {
		t.Fatal(err)
	}
	got := calculateProcessUsage(
		map[string]procSnapshot{"api": current},
		map[string]procSnapshot{"api": previous},
		[]containerSpec{{
			Name:      "api",
			Resources: resourceRequirements{Limits: map[string]string{"memory": "512Mi"}},
		}},
	)["api"]
	if len(got) != 2 || got[0].PID != 42 || got[0].Command != "worker" {
		t.Fatalf("unexpected process order: %#v", got)
	}
	if math.Abs(got[0].CPU-200) > 0.001 {
		t.Fatalf("worker CPU = %.3f%%, want 200%% (two cores)", got[0].CPU)
	}
	if math.Abs(got[0].Memory-0.78125) > 0.001 {
		t.Fatalf("worker memory = %.3f%% of limit, want 0.781%%", got[0].Memory)
	}
}

func TestProcCollectorUsesOnlyShellBuiltinsAndProcfs(t *testing.T) {
	if strings.Contains(procSnapshotCommand, "ps ") {
		t.Fatalf("collector must not depend on ps:\n%s", procSnapshotCommand)
	}
	for _, expected := range []string{"/proc/stat", "/stat", "/status", "/comm"} {
		if !strings.Contains(procSnapshotCommand, expected) {
			t.Fatalf("collector is missing %q:\n%s", expected, procSnapshotCommand)
		}
	}
	command := exec.Command("sh", "-n")
	command.Stdin = strings.NewReader(procSnapshotCommand)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("collector is not valid sh: %v: %s", err, output)
	}
}

func TestRenderSingleUsesCompactDashboardAndProcessAttribution(t *testing.T) {
	item := pod{}
	item.Metadata.Name = "example-api-a"
	item.Metadata.CreationTimestamp = time.Now().Add(-time.Minute)
	item.Status.Phase = "Running"
	item.Spec.NodeName = "example-node"
	item.Spec.Containers = []containerSpec{{
		Name: "api",
		Resources: resourceRequirements{
			Requests: map[string]string{"cpu": "100m", "memory": "128Mi"},
			Limits:   map[string]string{"cpu": "1", "memory": "512Mi"},
		},
	}}
	item.Status.ContainerStatuses = []containerStatus{{
		Name: "api", Ready: true, State: containerState{Running: &containerStateRunning{}},
	}}
	metrics := map[string]usage{"api": {CPU: 40, RAM: 64 * 1024 * 1024}}
	got := render(
		config{environment: "stage", namespace: "example", refresh: 5 * time.Second, processRefresh: 5 * time.Second, processes: true},
		"deployment/example-api",
		"",
		nil,
		snapshot{
			Pod: item, Metrics: metrics, Sample: makeSample(item, metrics),
			Processes: map[string][]processInfo{
				"api": {{PID: 42, PPID: 1, CPU: 2, Memory: 4, RSSKiB: 4096, Command: "worker"}},
			},
		},
	)
	for _, expected := range []string{
		"KUBERNETES POD ANALYTICS", "1 pod", "deployment/example-api", "CPU NOW",
		"PROCESS ATTRIBUTION", "PROC CPU", "CTR CPU", "20m", "40m", "worker[42]",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in:\n%s", expected, got)
		}
	}
	for _, legacy := range []string{"CONTAINERS · usage / request / limit", "LIM-MEM%", "RECENT EVENTS"} {
		if strings.Contains(got, legacy) {
			t.Fatalf("single-pod dashboard retained legacy section %q:\n%s", legacy, got)
		}
	}
}

func TestRenderSingleKeepsCompactHistoryWithoutExposingCSVPath(t *testing.T) {
	snap := healthySnapshot("example-api-a", 40, 64*1024*1024)
	now := time.Now()
	history := []sample{
		{At: now.Add(-time.Minute), PodUID: "old", CPU: 20, RAM: 32 * 1024 * 1024, Ready: 1, Total: 1},
		{At: now, PodUID: "new", CPU: 40, RAM: 64 * 1024 * 1024, Ready: 1, Total: 1},
	}
	got := render(
		config{environment: "stage", namespace: "example", refresh: time.Second, historyWindow: time.Hour, chartPoints: 24},
		"deployment/example-api", "/private/host/path/pod.csv", history, snap,
	)
	for _, expected := range []string{"HISTORY · local samples", "NOW", "AVG", "P95", "WINDOW", "2 samples", "2 pod UID(s)"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in compact history:\n%s", expected, got)
		}
	}
	if strings.Contains(got, "CSV:") || strings.Contains(got, "/private/host/path") {
		t.Fatalf("dashboard exposed an implementation path:\n%s", got)
	}
}

func TestProcessCPUShowsSamplingUntilSecondProcSnapshot(t *testing.T) {
	if got := formatProcessPercent(-1, true); got != "sample" {
		t.Fatalf("first process CPU sample = %q, want sample", got)
	}
}

func TestRenderMultiGroupsReplicasIntoCompactWorkloadDashboard(t *testing.T) {
	analyzers := []*analyzer{
		{cfg: config{pod: "example-api-a"}, series: "deployment/example-api"},
		{cfg: config{pod: "example-api-b"}, series: "deployment/example-api"},
	}
	results := []refreshResult{
		{snapshot: healthySnapshot("example-api-a", 25, 128*1024*1024)},
		{snapshot: healthySnapshot("example-api-b", 40, 160*1024*1024)},
	}
	got := renderMulti(config{
		environment: "stage", namespace: "example", refresh: 5 * time.Second, color: true,
	}, analyzers, results)
	if strings.Count(got, "deployment/example-api") != 1 {
		t.Fatalf("workload group must be rendered once:\n%s", got)
	}
	for _, expected := range []string{
		"2/2 pods ready", "example-api-a", "example-api-b", "CPU 65m", "33% req", "3% lim",
		"RAM 288.0Mi", "113% req", "28% lim", "CPU NOW", "CPU/REQ", "CPU/LIM",
		"RAM NOW", "RAM/REQ", "RAM/LIM", "│", colorGreen, colorMagenta,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in:\n%s", expected, got)
		}
	}
	if strings.Contains(got, "↳") {
		t.Fatalf("single-container pods must not render redundant nested rows:\n%s", got)
	}
	for _, noisy := range []string{"HISTORY", "RECENT EVENTS", strings.Repeat("═", 100)} {
		if strings.Contains(got, noisy) {
			t.Fatalf("multi dashboard contains noisy section %q:\n%s", noisy, got)
		}
	}
}

func TestRenderMultiCollapsesUnavailableProcCollectorIntoOneReadableNotice(t *testing.T) {
	analyzers := []*analyzer{
		{cfg: config{pod: "example-api-a"}, series: "deployment/example-api"},
		{cfg: config{pod: "example-api-b"}, series: "deployment/example-api"},
	}
	first := healthySnapshot("example-api-a", 10, 64*1024*1024)
	first.ProcessErrors = map[string]string{"api": "command terminated with exit code 127"}
	second := healthySnapshot("example-api-b", 10, 64*1024*1024)
	second.ProcessErrors = map[string]string{"api": "command terminated with exit code 127"}
	got := renderMulti(config{
		environment: "stage", namespace: "example", refresh: 5 * time.Second, processes: true,
	}, analyzers, []refreshResult{{snapshot: first}, {snapshot: second}})
	if strings.Contains(got, "exit code 127") {
		t.Fatalf("raw exec error leaked into dashboard:\n%s", got)
	}
	if strings.Count(got, "shell-based /proc collector is unavailable in the container image") != 1 {
		t.Fatalf("expected one collapsed process notice:\n%s", got)
	}
	if !strings.Contains(got, "2 containers") {
		t.Fatalf("missing collapsed container count:\n%s", got)
	}
}

func TestRenderMultiProcessesComparesProcTotalsWithContainerMetrics(t *testing.T) {
	analyzers := []*analyzer{{cfg: config{pod: "example-api-a"}, series: "deployment/example-api"}}
	snap := healthySnapshot("example-api-a", 40, 64*1024*1024)
	snap.Processes = map[string][]processInfo{
		"api": {
			{PID: 42, PPID: 1, CPU: 2, Memory: 1, RSSKiB: 4096, Command: "python"},
			{PID: 43, PPID: 1, CPU: 1, Memory: 1, RSSKiB: 2048, Command: "python"},
		},
	}
	got := renderMulti(config{
		environment: "stage", namespace: "example", refresh: time.Second, processes: true,
	}, analyzers, []refreshResult{{snapshot: snap}})
	for _, expected := range []string{
		"PROCESS ATTRIBUTION", "PROCS", "PROC CPU", "CTR CPU", "HOT CPU",
		"PROC RSS*", "CTR RAM", "30m", "40m", "20m", "6.0Mi", "64.0Mi", "python[42]",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in process attribution:\n%s", expected, got)
		}
	}
	if strings.Contains(got, "LIM-MEM%") {
		t.Fatalf("multi-pod process view must not repeat ambiguous per-process limit percentages:\n%s", got)
	}
}

func TestComposeDashboardKeepsSummaryAndProcessTableOnOneScreen(t *testing.T) {
	core := "HEADER\nSUMMARY\nWORKLOAD\nPOD TABLE\npod row\n"
	issues := "\nISSUES\nERROR first\nWARN second\nERROR third\nWARN fourth\nERROR fifth\nWARN sixth\n"
	events := "\nWARNING EVENTS\nWARN first\nWARN second\nWARN third\nWARN fourth\n"
	processes := "\nPROCESS ATTRIBUTION\nPOD CONTAINER PROCS PROC CPU CTR CPU\npod api 5 1.0 1.1\nREAD process details\nWARN process collector\n"
	footer := "\nCoverage details\n"

	got := composeDashboard(config{terminalLines: 19}, core, issues, events, processes, "", footer)
	if lines := dashboardLineCount(got); lines > 18 {
		t.Fatalf("dashboard uses %d lines, want at most 18:\n%s", lines, got)
	}
	for _, expected := range []string{"HEADER", "POD TABLE", "pod row", "ISSUES", "PROCESS ATTRIBUTION", "pod api 5", "more rows hidden"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in compact dashboard:\n%s", expected, got)
		}
	}
	if strings.Contains(got, "Coverage details") {
		t.Fatalf("low-priority footer must yield before the summary table scrolls:\n%s", got)
	}
}

func TestRenderMultiAggregatesReplicaPressureWarnings(t *testing.T) {
	analyzers := []*analyzer{
		{cfg: config{pod: "example-api-a"}, series: "deployment/example-api"},
		{cfg: config{pod: "example-api-b"}, series: "deployment/example-api"},
	}
	first := healthySnapshot("example-api-a", 10, 150*1024*1024)
	second := healthySnapshot("example-api-b", 10, 200*1024*1024)
	got := renderMulti(config{
		environment: "stage", namespace: "example", refresh: 5 * time.Second,
	}, analyzers, []refreshResult{{snapshot: first}, {snapshot: second}})
	if strings.Count(got, "memory usage") != 1 {
		t.Fatalf("replica warnings were not aggregated:\n%s", got)
	}
	for _, expected := range []string{"WARN  x2", "117–156% of request"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in aggregated warning:\n%s", expected, got)
		}
	}
	if !strings.Contains(got, "deployment/example-api  READY") {
		t.Fatalf("resource pressure changed lifecycle state:\n%s", got)
	}
}

func TestLifecycleStateStaysReadyDuringResourcePressure(t *testing.T) {
	snap := healthySnapshot("example-api-a", 950, 500*1024*1024)
	if got := podSnapshotState(snap, nil); got != "READY" {
		t.Fatalf("lifecycle state = %q, want READY; resource pressure belongs in ISSUES", got)
	}
	findings := analyze(snap)
	foundPressure := false
	for _, item := range findings {
		if item.Level == "ERROR" && strings.Contains(item.Message, "of limit") {
			foundPressure = true
		}
	}
	if !foundPressure {
		t.Fatal("expected resource pressure to remain visible as an issue")
	}
}

func TestRenderMultiSeparatesAndAggregatesOnlyRecentWarningEvents(t *testing.T) {
	now := time.Now()
	analyzers := []*analyzer{
		{cfg: config{pod: "example-api-a"}, series: "deployment/example-api"},
		{cfg: config{pod: "example-api-b"}, series: "deployment/example-api"},
	}
	first := healthySnapshot("example-api-a", 10, 64*1024*1024)
	first.Events = []event{{
		Type: "Warning", Reason: "Unhealthy", Count: 1, LastTimestamp: now.Add(-time.Minute),
		Message: `Readiness probe failed: Get "http://10.0.0.1:8000/readiness": timeout`,
	}}
	second := healthySnapshot("example-api-b", 10, 64*1024*1024)
	second.Events = []event{
		{
			Type: "Warning", Reason: "Unhealthy", Count: 2, LastTimestamp: now.Add(-2 * time.Minute),
			Message: `Readiness probe failed: Get "http://10.0.0.2:8000/readiness": timeout`,
		},
		{
			Type: "Warning", Reason: "OldWarning", Count: 5, LastTimestamp: now.Add(-20 * time.Minute),
			Message: "old warning",
		},
	}
	got := renderMulti(config{
		environment: "stage", namespace: "example", refresh: time.Second,
	}, analyzers, []refreshResult{{snapshot: first}, {snapshot: second}})
	if strings.Contains(got, "OldWarning") || strings.Contains(got, "10.0.0.") {
		t.Fatalf("stale event or pod address leaked into current dashboard:\n%s", got)
	}
	for _, expected := range []string{"WARNING EVENTS", "http://<pod>/readiness", "x3"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in grouped warning events:\n%s", expected, got)
		}
	}
}

func healthySnapshot(name string, cpu, memory float64) snapshot {
	item := pod{}
	item.Metadata.Name = name
	item.Metadata.CreationTimestamp = time.Now().Add(-time.Minute)
	item.Status.Phase = "Running"
	item.Spec.NodeName = "example-node"
	item.Spec.Containers = []containerSpec{{
		Name: "api",
		Resources: resourceRequirements{
			Requests: map[string]string{"cpu": "100m", "memory": "128Mi"},
			Limits:   map[string]string{"cpu": "1", "memory": "512Mi"},
		},
	}}
	item.Status.ContainerStatuses = []containerStatus{{
		Name: "api", Ready: true, State: containerState{Running: &containerStateRunning{}},
	}}
	metrics := map[string]usage{"api": {CPU: cpu, RAM: memory}}
	return snapshot{Pod: item, Metrics: metrics, Sample: makeSample(item, metrics)}
}

func TestParseTopOutput(t *testing.T) {
	got, err := parseTopOutput("example-api api 125m 256Mi\nexample-api sidecar 500000u 1Gi\n")
	if err != nil {
		t.Fatal(err)
	}
	if got["api"].CPU != 125 || got["api"].RAM != 256*1024*1024 {
		t.Fatalf("unexpected api metrics: %#v", got["api"])
	}
	if got["sidecar"].CPU != 500 || got["sidecar"].RAM != 1024*1024*1024 {
		t.Fatalf("unexpected sidecar metrics: %#v", got["sidecar"])
	}
}

func TestParseQuantities(t *testing.T) {
	cpu, err := parseCPU("1.5")
	if err != nil || cpu != 1500 {
		t.Fatalf("parseCPU = %v, %v", cpu, err)
	}
	ram, err := parseBytes("1.5Gi")
	if err != nil || ram != 1.5*1024*1024*1024 {
		t.Fatalf("parseBytes = %v, %v", ram, err)
	}
}

func TestSparklineAndPercentile(t *testing.T) {
	if got := sparkline([]float64{1, 2, 3, 4, 5}, 8); got != "▁▃▅▆█" {
		t.Fatalf("unexpected sparkline: %q", got)
	}
	if got := percentile([]float64{100, 1, 3, 2, 4}, 0.95); got != 100 {
		t.Fatalf("unexpected p95: %v", got)
	}
	if got := downsample([]float64{1, 3, 5, 7}, 2); len(got) != 2 || got[0] != 2 || got[1] != 6 {
		t.Fatalf("unexpected downsample: %#v", got)
	}
}

func TestHistoryRoundTripAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev", "pod.csv")
	now := time.Now().UTC()
	want := []sample{
		{At: now.Add(-2 * time.Hour), PodName: "old", PodUID: "1", CPU: 10, RAM: 20, Ready: 1, Total: 1},
		{At: now, PodName: "new", PodUID: "2", CPU: 30, RAM: 40, Restarts: 1, Ready: 1, Total: 1},
	}
	for _, item := range want {
		if err := appendHistory(path, item); err != nil {
			t.Fatal(err)
		}
	}
	got, err := loadHistory(path, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PodName != "new" || math.Abs(got[0].CPU-30) > 0.001 {
		t.Fatalf("unexpected retained history: %#v", got)
	}
}

func TestAnalyzeFindsOOMAndPressure(t *testing.T) {
	item := pod{}
	item.Status.Phase = "Running"
	item.Spec.Containers = []containerSpec{{
		Name: "api",
		Resources: resourceRequirements{
			Requests: map[string]string{"cpu": "100m", "memory": "128Mi"},
			Limits:   map[string]string{"cpu": "200m", "memory": "256Mi"},
		},
	}}
	item.Status.ContainerStatuses = []containerStatus{{
		Name: "api", Ready: true, RestartCount: 2,
		State:     containerState{Running: &containerStateRunning{}},
		LastState: containerState{Terminated: &containerStateTerminated{Reason: "OOMKilled", ExitCode: 137}},
	}}
	got := analyze(snapshot{Pod: item, Metrics: map[string]usage{"api": {CPU: 190, RAM: 250 * 1024 * 1024}}})
	var messages []string
	for _, item := range got {
		messages = append(messages, item.Level+" "+item.Message)
	}
	joined := strings.Join(messages, "\n")
	for _, expected := range []string{"OOMKilled", "CPU usage is 95% of limit", "memory usage is 98% of limit"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in:\n%s", expected, joined)
		}
	}
}

func TestResolveStableOwner(t *testing.T) {
	controller := true
	owner, ok := controllingOwner([]ownerReference{
		{Kind: "Noise", Name: "first"},
		{Kind: "Deployment", Name: "api", Controller: &controller},
	})
	if !ok || owner.Kind != "Deployment" || owner.Name != "api" {
		t.Fatalf("unexpected owner: %#v, %v", owner, ok)
	}
}
