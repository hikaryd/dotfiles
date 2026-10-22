package podanalyze

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestReadinessGatesAreRequired(t *testing.T) {
	var s snapshot
	s.Pod.Status.Phase = "Running"
	s.Sample = sample{Ready: 1, Total: 1}
	s.Pod.Status.Conditions = []podCondition{{Type: "Ready", Status: "False"}}
	if podSnapshotReady(s) {
		t.Fatal("readiness gate ignored")
	}
	s.Pod.Status.Conditions[0].Status = "True"
	if !podSnapshotReady(s) {
		t.Fatal("ready pod rejected")
	}
}
func TestHistorySeparatesPinnedTargets(t *testing.T) {
	a := config{historyDir: t.TempDir(), environment: "stage", namespace: "app", context: "a", kubeconfig: "one"}
	b := a
	b.context = "b"
	if historyFile(a, "deployment/api") == historyFile(b, "deployment/api") {
		t.Fatal("contexts share history")
	}
	b = a
	b.kubeconfig = "two"
	if historyFile(a, "deployment/api") == historyFile(b, "deployment/api") {
		t.Fatal("source chains share history")
	}
}
func TestHistoryUsesInheritedSourceChain(t *testing.T) {
	args := []string{"--context", "dots-stage", "--environment", "stage", "--namespace", "app", "--pod", "api"}
	path := func(extra ...string) string {
		cfg, err := parseConfig(append(append([]string{}, args...), extra...))
		if err != nil {
			t.Fatal(err)
		}
		if len(extra) == 0 && cfg.kubeconfig != "" {
			t.Fatal("environment chain must not become a single-file flag")
		}
		return historyFile(cfg, "deployment/api")
	}
	t.Setenv("KUBECONFIG", "/tmp/cluster-a.yaml:/tmp/shared.yaml")
	a := path()
	explicit := path("--kubeconfig", "/tmp/explicit.yaml")
	t.Setenv("KUBECONFIG", "/tmp/cluster-b.yaml:/tmp/shared.yaml")
	if a == path() {
		t.Fatal("inherited source chains share history")
	}
	if explicit != path("--kubeconfig", "/tmp/explicit.yaml") {
		t.Fatal("explicit source does not override environment")
	}
	t.Setenv("KUBECONFIG", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if path() != path("--kubeconfig", filepath.Join(home, ".kube", "config")) {
		t.Fatal("default source identity differs from explicit source")
	}
}

func TestKubernetesMemoryQuantities(t *testing.T) {
	for raw, want := range map[string]float64{"129k": 129000, "400m": 0.4, "1Pi": 1125899906842624, "1e3": 1000} {
		got, err := parseBytes(raw)
		if err != nil || got != want {
			t.Errorf("%s: %g %v", raw, got, err)
		}
	}
}
func TestEventControlsRemovedBeforeStyling(t *testing.T) {
	text := compactEventMessage("oops\x1b]52;c;value\a\x1b[31mRED")
	if strings.ContainsRune(text, '\x1b') {
		t.Fatal("terminal controls survived")
	}
}

func fakeAnalyzer(t *testing.T) *analyzer {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\ncase \"$*\" in *\"get pod/\"*) printf '%s\\n' '{\"metadata\":{\"name\":\"api\",\"uid\":\"one\"},\"status\":{\"phase\":\"Running\"}}';; *\"get events\"*) printf '%s\\n' '{\"items\":[]}';; esac\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return &analyzer{cfg: config{kubectl: path, pod: "api", retention: time.Hour}, historyPath: filepath.Join(dir, "history.csv"), historyPruned: time.Now()}
}
func TestHistoryFailureWarnsWithoutFreezingMemory(t *testing.T) {
	a := fakeAnalyzer(t)
	if err := os.Mkdir(a.historyPath, 0700); err != nil {
		t.Fatal(err)
	}
	snap, err := a.collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.HistoryError == "" || len(a.history) != 1 {
		t.Fatalf("warning=%q memory samples=%d", snap.HistoryError, len(a.history))
	}
	found := false
	for _, f := range analyze(snap) {
		if strings.Contains(f.Message, "history:") {
			found = true
		}
	}
	if !found {
		t.Fatal("history warning hidden")
	}
}
func TestHistoryPrunesDuringRunningSession(t *testing.T) {
	a := fakeAnalyzer(t)
	a.historyPruned = time.Now().Add(-6 * time.Minute)
	old := sample{At: time.Now().Add(-2 * time.Hour), PodUID: "expired"}
	if err := appendHistory(a.historyPath, old); err != nil {
		t.Fatal(err)
	}
	a.history = []sample{old}
	if _, err := a.collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, err := loadHistory(a.historyPath, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || len(a.history) != 1 || history[0].PodUID == "expired" {
		t.Fatalf("disk=%v memory=%v", history, a.history)
	}
}
func TestConcurrentHistoryTransactionsPreserveAcknowledgedSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.csv")
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			item := sample{At: time.Now(), PodUID: fmt.Sprint(i)}
			deadline := time.Now().Add(5 * time.Second)
			for {
				_, err := updateHistory(path, time.Time{}, &item, i%2 == 0)
				if err == nil {
					return
				}
				if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
					t.Errorf("transaction %d: %v", i, err)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}(i)
	}
	wg.Wait()
	history, err := loadHistory(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range history {
		seen[s.PodUID] = true
	}
	if len(history) != 40 || len(seen) != 40 {
		t.Fatalf("samples=%d unique=%d", len(history), len(seen))
	}
}
