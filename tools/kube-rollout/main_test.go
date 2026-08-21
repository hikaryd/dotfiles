package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDefaultRefreshInterval(t *testing.T) {
	if defaultRefresh != 100*time.Millisecond {
		t.Fatalf("default refresh = %s, want 100ms", defaultRefresh)
	}
}

func TestLabelSelectorIsStable(t *testing.T) {
	got := labelSelector(map[string]string{
		"track": "stable",
		"app":   "example",
	}, []labelSelectorRequirement{
		{Key: "tier", Operator: "In", Values: []string{"worker", "api"}},
	})
	if got != "app=example,tier in (api,worker),track=stable" {
		t.Fatalf("unexpected selector: %q", got)
	}
}

func TestStringListAcceptsRepeatedResources(t *testing.T) {
	var resources stringList
	for _, resource := range []string{
		"deployment/example-api",
		"deployment/example-worker",
	} {
		if err := resources.Set(resource); err != nil {
			t.Fatalf("Set(%q): %v", resource, err)
		}
	}
	if got := resources.String(); got != "deployment/example-api,deployment/example-worker" {
		t.Fatalf("unexpected resources: %q", got)
	}
}

func TestWorkloadCollectionAcceptsSingleObjectAndList(t *testing.T) {
	tests := []struct {
		name string
		json string
		want int
	}{
		{"single", `{"kind":"Deployment","metadata":{"name":"example-api"}}`, 1},
		{"list", `{"kind":"List","items":[{"kind":"Deployment","metadata":{"name":"example-api"}},{"kind":"Deployment","metadata":{"name":"example-worker"}}]}`, 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got workloadCollection
			if err := got.UnmarshalJSON([]byte(test.json)); err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != test.want {
				t.Fatalf("got %d items, want %d", len(got.Items), test.want)
			}
		})
	}
}

func TestMatchesSelector(t *testing.T) {
	labels := map[string]string{"app": "worker", "shard": "3", "track": "stable"}
	if !matchesSelector(
		labels,
		map[string]string{"app": "worker"},
		[]labelSelectorRequirement{
			{Key: "track", Operator: "In", Values: []string{"stable", "canary"}},
			{Key: "shard", Operator: "Gt", Values: []string{"2"}},
		},
	) {
		t.Fatal("expected selector to match")
	}
	if matchesSelector(
		labels,
		nil,
		[]labelSelectorRequirement{{Key: "track", Operator: "NotIn", Values: []string{"stable"}}},
	) {
		t.Fatal("expected selector not to match")
	}
	if !matchesSelector(
		map[string]string{"app": "worker"},
		nil,
		[]labelSelectorRequirement{{Key: "track", Operator: "NotIn", Values: []string{"stable"}}},
	) {
		t.Fatal("NotIn should match when the key is absent")
	}
	if matchesSelector(
		map[string]string{"app": "worker"},
		map[string]string{"track": ""},
		nil,
	) {
		t.Fatal("matchLabels should not treat an absent key as an empty value")
	}
}

func TestSummarizeHealthyDeployment(t *testing.T) {
	replicas := 3
	item := workload{Kind: "Deployment"}
	item.Metadata.Generation = 7
	item.Spec.Replicas = &replicas
	item.Status.ObservedGeneration = 7
	item.Status.UpdatedReplicas = 3
	item.Status.ReadyReplicas = 3
	item.Status.AvailableReplicas = 3

	got := summarize(item, nil)
	if got.State != "OK" {
		t.Fatalf("expected OK, got %#v", got)
	}
}

func TestSummarizeDetectsCrashLoop(t *testing.T) {
	replicas := 1
	item := workload{Kind: "Deployment"}
	item.Metadata.Generation = 2
	item.Spec.Replicas = &replicas
	item.Status.ObservedGeneration = 2
	item.Status.UpdatedReplicas = 1

	failing := pod{}
	failing.Metadata.Name = "example-api-bad"
	failing.Status.Phase = "Running"
	failing.Status.ContainerStatuses = []containerStatus{
		{
			Name: "api",
			State: containerState{
				Waiting: &struct {
					Reason  string `json:"reason"`
					Message string `json:"message"`
				}{Reason: "CrashLoopBackOff"},
			},
		},
	}

	got := summarize(item, []pod{failing})
	if got.State != "DEGRADED" {
		t.Fatalf("expected DEGRADED, got %#v", got)
	}
}

func TestAggregateSummariesUsesWorstStateAndTotals(t *testing.T) {
	healthy := workload{}
	healthy.Metadata.Name = "example-api"
	rolling := workload{}
	rolling.Metadata.Name = "example-worker"

	got := aggregateSummaries([]workloadView{
		{
			workload: healthy,
			summary: rolloutSummary{
				State: "OK", Desired: 2, Updated: 2, Ready: 2, Available: 2,
			},
		},
		{
			workload: rolling,
			summary: rolloutSummary{
				State: "ROLLING", Desired: 1, Updated: 1,
			},
		},
	})
	if got.State != "ROLLING" || got.Desired != 3 || got.Ready != 2 {
		t.Fatalf("unexpected aggregate: %#v", got)
	}
	if !strings.Contains(got.Message, "example-worker: ROLLING") {
		t.Fatalf("unexpected aggregate message: %q", got.Message)
	}
}

func TestPodDisplayStatusShowsTermination(t *testing.T) {
	now := time.Now()
	item := pod{}
	item.Metadata.DeletionTimestamp = &now
	item.Status.Phase = "Running"

	if got := podDisplayStatus(item); got != "Terminating" {
		t.Fatalf("expected Terminating, got %q", got)
	}
}

func TestAppendBoundedKeepsNewestItems(t *testing.T) {
	got := []int{}
	for i := 1; i <= 5; i++ {
		got = appendBounded(got, i, 3)
	}
	want := []int{3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLogColorUsesSeverity(t *testing.T) {
	tests := map[string]string{
		`{"level":"info","message":"started"}`: colorGreen,
		`{"level":"warning","message":"slow"}`: colorYellow,
		`panic: worker failed`:                 colorRed,
		`plain application output`:             "",
	}
	for line, want := range tests {
		if got := logColor(line); got != want {
			t.Errorf("logColor(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestRenderLogTailWrapsLongLinesInsteadOfTruncatingThem(t *testing.T) {
	var out strings.Builder
	renderLogTail(&out, []logLine{{
		pod:  "example-api-a",
		text: "1234567890123456712345678901234567TAILEND",
		at:   time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
	}}, 4, 56)
	got := out.String()
	if !strings.Contains(got, "TAILEND") {
		t.Fatalf("long log tail was lost: %q", got)
	}
	if strings.Contains(got, "…") {
		t.Fatalf("long log line was truncated: %q", got)
	}
}

func TestReadOnlyGuardRejectsMutation(t *testing.T) {
	d := dashboard{}
	err := d.kubectlJSON(context.Background(), &map[string]any{}, "delete", "pod/example")
	if err == nil || !strings.Contains(err.Error(), "read-only guard") {
		t.Fatalf("expected read-only guard error, got %v", err)
	}
}
