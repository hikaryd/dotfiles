package main

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
