package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultRefresh = 100 * time.Millisecond

	colorReset   = "\x1b[0m"
	colorDim     = "\x1b[2m"
	colorRed     = "\x1b[31m"
	colorGreen   = "\x1b[32m"
	colorYellow  = "\x1b[33m"
	colorMagenta = "\x1b[35m"
	colorCyan    = "\x1b[36m"
	colorBold    = "\x1b[1m"
)

const procSnapshotCommand = `
IFS=' ' read -r _ user nice system idle iowait irq softirq steal _ < /proc/stat || exit 1
total=$((user + nice + system + idle + iowait + irq + softirq + steal))
cores=0
while IFS=' ' read -r label _; do
  case "$label" in cpu[0-9]*) cores=$((cores + 1));; esac
done < /proc/stat
printf '@\t%s\t%s\n' "$total" "$cores"
for dir in /proc/[0-9]*; do
  [ -r "$dir/stat" ] || continue
  IFS= read -r stat < "$dir/stat" || continue
  rest=${stat##*) }
  set -- $rest
  [ "$#" -ge 13 ] || continue
  pid=${dir##*/}
  ppid=$2
  ticks=$((${12} + ${13}))
  rss=0
  if [ -r "$dir/status" ]; then
    while IFS=: read -r key value; do
      case "$key" in
        VmRSS) set -- $value; rss=${1:-0}; break;;
      esac
    done < "$dir/status"
  fi
  name="?"
  IFS= read -r name < "$dir/comm" || true
  printf '%s\t%s\t%s\t%s\t%s\n' "$pid" "$ppid" "$ticks" "$rss" "$name"
done`

type config struct {
	kubectl        string
	kubeconfig     string
	context        string
	namespace      string
	environment    string
	pods           stringList
	pod            string
	historyDir     string
	refresh        time.Duration
	historyWindow  time.Duration
	retention      time.Duration
	chartPoints    int
	processes      bool
	processRefresh time.Duration
	terminalLines  int
	once           bool
	live           bool
	color          bool
	multi          bool
}

type stringList []string

func (items *stringList) String() string {
	return strings.Join(*items, ",")
}

func (items *stringList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "/ \t\r\n") {
		return fmt.Errorf("invalid pod name %q", value)
	}
	for _, existing := range *items {
		if existing == value {
			return nil
		}
	}
	*items = append(*items, value)
	return nil
}

type ownerReference struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller *bool  `json:"controller"`
}

type objectMeta struct {
	Name              string            `json:"name"`
	UID               string            `json:"uid"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
	OwnerReferences   []ownerReference  `json:"ownerReferences"`
}

type resourceRequirements struct {
	Requests map[string]string `json:"requests"`
	Limits   map[string]string `json:"limits"`
}

type containerSpec struct {
	Name      string               `json:"name"`
	Image     string               `json:"image"`
	Resources resourceRequirements `json:"resources"`
}

type containerStateTerminated struct {
	ExitCode   int       `json:"exitCode"`
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	FinishedAt time.Time `json:"finishedAt"`
}

type containerStateWaiting struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type containerStateRunning struct {
	StartedAt time.Time `json:"startedAt"`
}

type containerState struct {
	Terminated *containerStateTerminated `json:"terminated"`
	Waiting    *containerStateWaiting    `json:"waiting"`
	Running    *containerStateRunning    `json:"running"`
}

type containerStatus struct {
	Name         string         `json:"name"`
	Ready        bool           `json:"ready"`
	RestartCount int            `json:"restartCount"`
	State        containerState `json:"state"`
	LastState    containerState `json:"lastState"`
}

type podCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

type pod struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		NodeName       string          `json:"nodeName"`
		Containers     []containerSpec `json:"containers"`
		InitContainers []containerSpec `json:"initContainers"`
	} `json:"spec"`
	Status struct {
		Phase                 string            `json:"phase"`
		Reason                string            `json:"reason"`
		Message               string            `json:"message"`
		PodIP                 string            `json:"podIP"`
		HostIP                string            `json:"hostIP"`
		QOSClass              string            `json:"qosClass"`
		StartTime             time.Time         `json:"startTime"`
		Conditions            []podCondition    `json:"conditions"`
		ContainerStatuses     []containerStatus `json:"containerStatuses"`
		InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
	} `json:"status"`
}

type kubernetesObject struct {
	Metadata objectMeta `json:"metadata"`
}

type event struct {
	Metadata      objectMeta `json:"metadata"`
	Type          string     `json:"type"`
	Reason        string     `json:"reason"`
	Message       string     `json:"message"`
	Count         int        `json:"count"`
	EventTime     time.Time  `json:"eventTime"`
	LastTimestamp time.Time  `json:"lastTimestamp"`
	Series        *struct {
		Count            int       `json:"count"`
		LastObservedTime time.Time `json:"lastObservedTime"`
	} `json:"series"`
}

type eventList struct {
	Items []event `json:"items"`
}

type usage struct {
	CPU float64
	RAM float64
}

type sample struct {
	At       time.Time
	PodName  string
	PodUID   string
	CPU      float64
	RAM      float64
	Restarts int
	Ready    int
	Total    int
}

type snapshot struct {
	Pod           pod
	Metrics       map[string]usage
	MetricsError  string
	Events        []event
	Sample        sample
	Processes     map[string][]processInfo
	ProcessErrors map[string]string
}

type processInfo struct {
	PID     int
	PPID    int
	CPU     float64
	Memory  float64
	RSSKiB  int64
	Command string
}

type procCounter struct {
	PID     int
	PPID    int
	Ticks   uint64
	RSSKiB  int64
	Command string
}

type procSnapshot struct {
	TotalTicks uint64
	Cores      int
	Processes  []procCounter
}

type finding struct {
	Level   string
	Message string
}

type analyzer struct {
	cfg           config
	series        string
	historyPath   string
	history       []sample
	events        []event
	eventsAt      time.Time
	processes     map[string][]processInfo
	processErrors map[string]string
	procSnapshots map[string]procSnapshot
	processesNext time.Time
	lastError     string
}

type refreshResult struct {
	snapshot snapshot
	err      error
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	live := shouldRunLive(cfg, isTerminal(os.Stdout))

	analyzers := make([]*analyzer, 0, len(cfg.pods))
	for _, podName := range cfg.pods {
		podConfig := cfg
		podConfig.pod = podName
		podConfig.multi = len(cfg.pods) > 1
		a, buildErr := newAnalyzer(ctx, podConfig)
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "cannot initialize pod/%s: %v\n", podName, buildErr)
			os.Exit(1)
		}
		analyzers = append(analyzers, a)
	}

	refreshAll := func() bool {
		allHealthy := true
		results := make([]refreshResult, len(analyzers))
		var group sync.WaitGroup
		for index, a := range analyzers {
			group.Add(1)
			go func(index int, a *analyzer) {
				defer group.Done()
				results[index].snapshot, results[index].err = a.collect(ctx)
			}(index, a)
		}
		group.Wait()

		if live {
			fmt.Print("\x1b[2J\x1b[H")
		}
		if len(analyzers) > 1 {
			for _, result := range results {
				if result.err != nil {
					allHealthy = false
				}
			}
			fmt.Print(renderMulti(cfg, analyzers, results))
			return allHealthy
		}
		for index, a := range analyzers {
			snap, refreshErr := results[index].snapshot, results[index].err
			if refreshErr != nil {
				allHealthy = false
				fmt.Printf("Kubernetes %s · %s · pod/%s · READ ONLY\n\n", a.cfg.environment, a.cfg.namespace, a.cfg.pod)
				fmt.Printf("ERROR: cannot refresh pod: %v\n", refreshErr)
				continue
			}
			fmt.Print(render(a.cfg, a.series, a.historyPath, a.history, snap))
		}
		return allHealthy
	}

	if !live {
		if !refreshAll() {
			os.Exit(1)
		}
		return
	}

	fmt.Print("\x1b[?1049h\x1b[?25l")
	defer fmt.Print("\x1b[?25h\x1b[?1049l")

	ticker := time.NewTicker(cfg.refresh)
	defer ticker.Stop()
	for {
		_ = refreshAll()
		select {
		case <-ctx.Done():
			fmt.Println()
			return
		case <-ticker.C:
		}
	}
}

func shouldRunLive(cfg config, terminal bool) bool {
	return !cfg.once && (cfg.live || terminal)
}

func newAnalyzer(ctx context.Context, cfg config) (*analyzer, error) {
	initial, err := fetchPod(ctx, cfg)
	if err != nil {
		return nil, err
	}
	series := resolveSeries(ctx, cfg, initial)
	historyPath := historyFile(cfg, series)
	var history []sample
	if !cfg.multi {
		history, err = loadHistory(historyPath, time.Now().Add(-cfg.retention))
		if err != nil {
			return nil, fmt.Errorf("load metrics history: %w", err)
		}
		if err := rewriteHistory(historyPath, history); err != nil {
			return nil, fmt.Errorf("prune metrics history: %w", err)
		}
	}
	return &analyzer{
		cfg:         cfg,
		series:      series,
		historyPath: historyPath,
		history:     history,
	}, nil
}

func parseConfig() (config, error) {
	home, _ := os.UserHomeDir()
	cfg := config{}
	flag.StringVar(&cfg.kubectl, "kubectl", "kubectl", "kubectl executable")
	flag.StringVar(&cfg.kubeconfig, "kubeconfig", "", "pinned kubeconfig")
	flag.StringVar(&cfg.context, "context", "", "pinned context")
	flag.StringVar(&cfg.namespace, "namespace", "", "namespace")
	flag.StringVar(&cfg.environment, "environment", "", "environment label")
	flag.Var(&cfg.pods, "pod", "pod name; repeat for a combined multi-pod view")
	flag.StringVar(&cfg.historyDir, "history-dir", filepath.Join(home, ".local", "state", "kube-tools", "pod-metrics"), "local metric history directory")
	flag.DurationVar(&cfg.refresh, "refresh", defaultRefresh, "live refresh interval")
	flag.DurationVar(&cfg.historyWindow, "history-window", 24*time.Hour, "history shown in charts")
	flag.DurationVar(&cfg.retention, "retention", 30*24*time.Hour, "local history retention")
	flag.IntVar(&cfg.chartPoints, "chart-points", 72, "maximum terminal chart points")
	flag.BoolVar(&cfg.processes, "processes", true, "show best-effort process CPU/RAM snapshots through read-only kubectl exec /proc sampling")
	flag.DurationVar(&cfg.processRefresh, "process-refresh", 5*time.Second, "process snapshot refresh interval")
	flag.IntVar(&cfg.terminalLines, "terminal-lines", detectTerminalLines(), "terminal height used to keep the live dashboard on one screen")
	flag.BoolVar(&cfg.once, "once", false, "render one snapshot and exit")
	flag.BoolVar(&cfg.live, "live", false, "force live dashboard mode when stdout TTY detection is unavailable")
	flag.Parse()
	terminal := isTerminal(os.Stdout)
	cfg.color = os.Getenv("NO_COLOR") == "" && (terminal || cfg.live && !cfg.once)

	var missing []string
	for name, value := range map[string]string{
		"--kubeconfig":  cfg.kubeconfig,
		"--context":     cfg.context,
		"--namespace":   cfg.namespace,
		"--environment": cfg.environment,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return cfg, fmt.Errorf("required flags are missing: %s", strings.Join(missing, ", "))
	}
	if len(cfg.pods) == 0 {
		return cfg, errors.New("required flags are missing: --pod")
	}
	if cfg.refresh < 100*time.Millisecond {
		return cfg, errors.New("--refresh must be at least 100ms")
	}
	if cfg.historyWindow <= 0 || cfg.retention <= 0 || cfg.retention < cfg.historyWindow {
		return cfg, errors.New("history window and retention must be positive; retention must cover the window")
	}
	if cfg.chartPoints < 8 {
		return cfg, errors.New("--chart-points must be at least 8")
	}
	if cfg.processRefresh <= 0 {
		return cfg, errors.New("--process-refresh must be positive")
	}
	if cfg.terminalLines < 0 {
		return cfg, errors.New("--terminal-lines cannot be negative")
	}
	return cfg, nil
}

func (a *analyzer) collect(ctx context.Context) (snapshot, error) {
	item, err := fetchPod(ctx, a.cfg)
	if err != nil {
		a.lastError = err.Error()
		return snapshot{}, err
	}

	metrics, metricsErr := fetchMetrics(ctx, a.cfg)
	if time.Since(a.eventsAt) >= 30*time.Second || a.eventsAt.IsZero() {
		if events, eventErr := fetchEvents(ctx, a.cfg); eventErr == nil {
			a.events = events
			a.eventsAt = time.Now()
		}
	}
	now := time.Now()
	if a.cfg.processes && (a.processesNext.IsZero() || !now.Before(a.processesNext)) {
		current, processErrors := fetchProcSnapshots(ctx, a.cfg, item)
		a.processes = calculateProcessUsage(current, a.procSnapshots, item.Spec.Containers)
		a.processErrors = processErrors
		a.procSnapshots = current
		a.processesNext = now.Add(a.cfg.processRefresh)
		if len(current) == 0 && len(a.processErrors) > 0 {
			a.processesNext = now.Add(max(a.cfg.processRefresh, 5*time.Minute))
		}
	}

	s := makeSample(item, metrics)
	if !a.cfg.multi {
		if err := appendHistory(a.historyPath, s); err != nil {
			a.lastError = "history: " + err.Error()
		} else {
			a.history = append(a.history, s)
		}
		a.history = trimSamples(a.history, time.Now().Add(-a.cfg.retention))
	}

	snap := snapshot{
		Pod: item, Metrics: metrics, Events: a.events, Sample: s,
		Processes: a.processes, ProcessErrors: a.processErrors,
	}
	if metricsErr != nil {
		snap.MetricsError = metricsErr.Error()
	}
	return snap, nil
}

func kubectlOutput(ctx context.Context, cfg config, args ...string) ([]byte, error) {
	base := []string{"--kubeconfig", cfg.kubeconfig, "--context", cfg.context, "--namespace", cfg.namespace}
	command := exec.CommandContext(ctx, cfg.kubectl, append(base, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}

func fetchPod(ctx context.Context, cfg config) (pod, error) {
	var item pod
	data, err := kubectlOutput(ctx, cfg, "get", "pod/"+cfg.pod, "-o", "json")
	if err != nil {
		return item, err
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return item, fmt.Errorf("decode pod JSON: %w", err)
	}
	return item, nil
}

func fetchMetrics(ctx context.Context, cfg config) (map[string]usage, error) {
	data, err := kubectlOutput(ctx, cfg, "top", "pod", cfg.pod, "--containers", "--no-headers")
	if err != nil {
		return nil, err
	}
	return parseTopOutput(string(data))
}

func fetchEvents(ctx context.Context, cfg config) ([]event, error) {
	data, err := kubectlOutput(ctx, cfg,
		"get", "events",
		"--field-selector", "involvedObject.kind=Pod,involvedObject.name="+cfg.pod,
		"-o", "json",
	)
	if err != nil {
		return nil, err
	}
	var list eventList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode events JSON: %w", err)
	}
	sort.Slice(list.Items, func(i, j int) bool {
		return eventTime(list.Items[i]).After(eventTime(list.Items[j]))
	})
	return list.Items, nil
}

func fetchProcSnapshots(ctx context.Context, cfg config, item pod) (map[string]procSnapshot, map[string]string) {
	snapshots := make(map[string]procSnapshot, len(item.Spec.Containers))
	errorsByContainer := make(map[string]string)
	for _, container := range item.Spec.Containers {
		data, err := kubectlOutput(ctx, cfg,
			"exec", "pod/"+cfg.pod, "-c", container.Name,
			"--", "sh", "-c", procSnapshotCommand,
		)
		if err != nil {
			errorsByContainer[container.Name] = err.Error()
			continue
		}
		item, err := parseProcSnapshot(string(data))
		if err != nil {
			errorsByContainer[container.Name] = err.Error()
			continue
		}
		snapshots[container.Name] = item
	}
	return snapshots, errorsByContainer
}

func parseProcSnapshot(output string) (procSnapshot, error) {
	var result procSnapshot
	for lineNumber, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if fields[0] == "@" {
			if len(fields) != 3 {
				return result, fmt.Errorf("unexpected /proc header: %q", line)
			}
			var err error
			if result.TotalTicks, err = strconv.ParseUint(fields[1], 10, 64); err != nil {
				return result, fmt.Errorf("parse total CPU ticks: %w", err)
			}
			if result.Cores, err = strconv.Atoi(fields[2]); err != nil || result.Cores <= 0 {
				return result, fmt.Errorf("parse CPU count %q", fields[2])
			}
			continue
		}
		if len(fields) != 5 {
			return result, fmt.Errorf("unexpected /proc row %d: %q", lineNumber+1, line)
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return result, fmt.Errorf("parse PID in /proc row %d: %w", lineNumber+1, err)
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			return result, fmt.Errorf("parse PPID in /proc row %d: %w", lineNumber+1, err)
		}
		ticks, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return result, fmt.Errorf("parse CPU ticks in /proc row %d: %w", lineNumber+1, err)
		}
		rss, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return result, fmt.Errorf("parse RSS in /proc row %d: %w", lineNumber+1, err)
		}
		result.Processes = append(result.Processes, procCounter{
			PID: pid, PPID: ppid, Ticks: ticks, RSSKiB: rss, Command: fields[4],
		})
	}
	if result.TotalTicks == 0 || result.Cores == 0 {
		return result, errors.New("/proc snapshot is missing CPU totals")
	}
	if len(result.Processes) == 0 {
		return result, errors.New("/proc returned no process rows")
	}
	return result, nil
}

func calculateProcessUsage(current, previous map[string]procSnapshot, containers []containerSpec) map[string][]processInfo {
	result := make(map[string][]processInfo, len(current))
	limits := make(map[string]float64, len(containers))
	for _, container := range containers {
		if limit, err := parseBytes(container.Resources.Limits["memory"]); err == nil && limit > 0 {
			limits[container.Name] = limit
		}
	}
	for container, snapshot := range current {
		old := previous[container]
		oldByPID := make(map[int]procCounter, len(old.Processes))
		for _, process := range old.Processes {
			oldByPID[process.PID] = process
		}
		totalDelta := snapshot.TotalTicks - min(snapshot.TotalTicks, old.TotalTicks)
		rows := make([]processInfo, 0, len(snapshot.Processes))
		for _, process := range snapshot.Processes {
			cpu := -1.0
			if prior, found := oldByPID[process.PID]; found && totalDelta > 0 && process.Ticks >= prior.Ticks {
				cpu = float64(process.Ticks-prior.Ticks) / float64(totalDelta) * float64(snapshot.Cores) * 100
			}
			memory := -1.0
			if limit := limits[container]; limit > 0 {
				memory = float64(process.RSSKiB) * 1024 / limit * 100
			}
			rows = append(rows, processInfo{
				PID: process.PID, PPID: process.PPID, CPU: cpu, Memory: memory,
				RSSKiB: process.RSSKiB, Command: process.Command,
			})
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].CPU == rows[j].CPU {
				return rows[i].RSSKiB > rows[j].RSSKiB
			}
			if rows[i].CPU < 0 {
				return false
			}
			if rows[j].CPU < 0 {
				return true
			}
			return rows[i].CPU > rows[j].CPU
		})
		result[container] = rows
	}
	return result
}

func resolveSeries(ctx context.Context, cfg config, item pod) string {
	owner, ok := controllingOwner(item.Metadata.OwnerReferences)
	if !ok {
		return "pod/" + item.Metadata.Name
	}
	if owner.Kind != "ReplicaSet" && owner.Kind != "Job" {
		return strings.ToLower(owner.Kind) + "/" + owner.Name
	}
	data, err := kubectlOutput(ctx, cfg, "get", strings.ToLower(owner.Kind)+"/"+owner.Name, "-o", "json")
	if err != nil {
		return strings.ToLower(owner.Kind) + "/" + owner.Name
	}
	var parent kubernetesObject
	if json.Unmarshal(data, &parent) != nil {
		return strings.ToLower(owner.Kind) + "/" + owner.Name
	}
	if top, found := controllingOwner(parent.Metadata.OwnerReferences); found {
		return strings.ToLower(top.Kind) + "/" + top.Name
	}
	return strings.ToLower(owner.Kind) + "/" + owner.Name
}

func controllingOwner(owners []ownerReference) (ownerReference, bool) {
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller {
			return owner, true
		}
	}
	if len(owners) > 0 {
		return owners[0], true
	}
	return ownerReference{}, false
}

func parseTopOutput(output string) (map[string]usage, error) {
	metrics := make(map[string]usage)
	for lineNumber, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return nil, fmt.Errorf("unexpected kubectl top row %d: %q", lineNumber+1, line)
		}
		cpu, err := parseCPU(fields[len(fields)-2])
		if err != nil {
			return nil, fmt.Errorf("parse CPU in row %d: %w", lineNumber+1, err)
		}
		ram, err := parseBytes(fields[len(fields)-1])
		if err != nil {
			return nil, fmt.Errorf("parse memory in row %d: %w", lineNumber+1, err)
		}
		container := fields[len(fields)-3]
		metrics[container] = usage{CPU: cpu, RAM: ram}
	}
	if len(metrics) == 0 {
		return nil, errors.New("kubectl top returned no container metrics")
	}
	return metrics, nil
}

func parseCPU(value string) (float64, error) {
	multiplier := 1000.0
	suffix := ""
	for _, candidate := range []struct {
		suffix string
		factor float64
	}{{"n", 0.000001}, {"u", 0.001}, {"m", 1}} {
		if strings.HasSuffix(value, candidate.suffix) {
			suffix = candidate.suffix
			multiplier = candidate.factor
			break
		}
	}
	number := strings.TrimSuffix(value, suffix)
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid CPU quantity %q", value)
	}
	return parsed * multiplier, nil
}

func parseBytes(value string) (float64, error) {
	factors := map[string]float64{
		"Ki": 1024, "Mi": 1024 * 1024, "Gi": 1024 * 1024 * 1024, "Ti": 1024 * 1024 * 1024 * 1024,
		"K": 1e3, "M": 1e6, "G": 1e9, "T": 1e12,
	}
	suffix := ""
	factor := 1.0
	for _, candidate := range []string{"Ki", "Mi", "Gi", "Ti", "K", "M", "G", "T"} {
		if strings.HasSuffix(value, candidate) {
			suffix = candidate
			factor = factors[candidate]
			break
		}
	}
	parsed, err := strconv.ParseFloat(strings.TrimSuffix(value, suffix), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory quantity %q", value)
	}
	return parsed * factor, nil
}

func makeSample(item pod, metrics map[string]usage) sample {
	s := sample{At: time.Now().UTC(), PodName: item.Metadata.Name, PodUID: item.Metadata.UID}
	for _, metric := range metrics {
		s.CPU += metric.CPU
		s.RAM += metric.RAM
	}
	for _, status := range item.Status.ContainerStatuses {
		s.Total++
		s.Restarts += status.RestartCount
		if status.Ready {
			s.Ready++
		}
	}
	if len(metrics) == 0 {
		s.CPU = -1
		s.RAM = -1
	}
	return s
}

func historyFile(cfg config, series string) string {
	return filepath.Join(
		cfg.historyDir,
		safePathPart(cfg.environment),
		safePathPart(cfg.namespace),
		safePathPart(strings.ReplaceAll(series, "/", "-"))+".csv",
	)
}

func safePathPart(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.", r) {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

var historyHeader = []string{"timestamp", "pod_name", "pod_uid", "cpu_millicores", "memory_bytes", "restarts", "ready", "total"}

func appendHistory(path string, item sample) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	info, statErr := os.Stat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	var writeErr error
	if errors.Is(statErr, os.ErrNotExist) || info.Size() == 0 {
		writeErr = writer.Write(historyHeader)
	}
	if writeErr == nil {
		writeErr = writer.Write(sampleRecord(item))
	}
	writer.Flush()
	if writeErr == nil {
		writeErr = writer.Error()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}

func rewriteHistory(path string, samples []sample) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	writeErr := writer.Write(historyHeader)
	for _, item := range samples {
		if writeErr == nil {
			writeErr = writer.Write(sampleRecord(item))
		}
	}
	writer.Flush()
	if writeErr == nil {
		writeErr = writer.Error()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(temporary)
		return writeErr
	}
	return os.Rename(temporary, path)
}

func sampleRecord(item sample) []string {
	return []string{
		item.At.Format(time.RFC3339Nano), item.PodName, item.PodUID,
		strconv.FormatFloat(item.CPU, 'f', 3, 64), strconv.FormatFloat(item.RAM, 'f', 0, 64),
		strconv.Itoa(item.Restarts), strconv.Itoa(item.Ready), strconv.Itoa(item.Total),
	}
}

func loadHistory(path string, cutoff time.Time) ([]sample, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	reader := csv.NewReader(file)
	var samples []sample
	for row := 0; ; row++ {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		if row == 0 && len(record) > 0 && record[0] == "timestamp" {
			continue
		}
		item, parseErr := parseSampleRecord(record)
		if parseErr != nil {
			return nil, fmt.Errorf("history row %d: %w", row+1, parseErr)
		}
		if !item.At.Before(cutoff) {
			samples = append(samples, item)
		}
	}
	return samples, nil
}

func parseSampleRecord(record []string) (sample, error) {
	var item sample
	if len(record) != len(historyHeader) {
		return item, fmt.Errorf("got %d columns, want %d", len(record), len(historyHeader))
	}
	var err error
	if item.At, err = time.Parse(time.RFC3339Nano, record[0]); err != nil {
		return item, err
	}
	item.PodName, item.PodUID = record[1], record[2]
	if item.CPU, err = strconv.ParseFloat(record[3], 64); err != nil {
		return item, err
	}
	if item.RAM, err = strconv.ParseFloat(record[4], 64); err != nil {
		return item, err
	}
	if item.Restarts, err = strconv.Atoi(record[5]); err != nil {
		return item, err
	}
	if item.Ready, err = strconv.Atoi(record[6]); err != nil {
		return item, err
	}
	if item.Total, err = strconv.Atoi(record[7]); err != nil {
		return item, err
	}
	return item, nil
}

func trimSamples(samples []sample, cutoff time.Time) []sample {
	first := sort.Search(len(samples), func(index int) bool { return !samples[index].At.Before(cutoff) })
	return samples[first:]
}

type multiGroup struct {
	series  string
	indices []int
}

type issueSummary struct {
	level      string
	message    string
	count      int
	percentage bool
	prefix     string
	suffix     string
	minPercent int
	maxPercent int
}

type processAttribution struct {
	pod          string
	container    string
	processCount int
	totalCPU     float64
	hasCPU       bool
	totalRSSKiB  int64
	hottest      processInfo
	containerUse usage
	hasUsage     bool
}

type warningEventSummary struct {
	reason  string
	message string
	count   int
	latest  time.Time
}

type resourceTotals struct {
	CPURequest         float64
	CPULimit           float64
	RAMRequest         float64
	RAMLimit           float64
	CPURequestComplete bool
	CPULimitComplete   bool
	RAMRequestComplete bool
	RAMLimitComplete   bool
}

func resourcesForPods(items []pod) resourceTotals {
	containers := make([]containerSpec, 0)
	for _, item := range items {
		containers = append(containers, item.Spec.Containers...)
	}
	return resourcesForContainers(containers)
}

func resourcesForContainers(containers []containerSpec) resourceTotals {
	totals := resourceTotals{
		CPURequestComplete: len(containers) > 0,
		CPULimitComplete:   len(containers) > 0,
		RAMRequestComplete: len(containers) > 0,
		RAMLimitComplete:   len(containers) > 0,
	}
	for _, container := range containers {
		accumulateResource(&totals.CPURequest, &totals.CPURequestComplete, container.Resources.Requests["cpu"], parseCPU)
		accumulateResource(&totals.CPULimit, &totals.CPULimitComplete, container.Resources.Limits["cpu"], parseCPU)
		accumulateResource(&totals.RAMRequest, &totals.RAMRequestComplete, container.Resources.Requests["memory"], parseBytes)
		accumulateResource(&totals.RAMLimit, &totals.RAMLimitComplete, container.Resources.Limits["memory"], parseBytes)
	}
	return totals
}

func accumulateResource(total *float64, complete *bool, value string, parser func(string) (float64, error)) {
	if value == "" {
		*complete = false
		return
	}
	parsed, err := parser(value)
	if err != nil || parsed <= 0 {
		*complete = false
		return
	}
	*total += parsed
}

func renderMulti(cfg config, analyzers []*analyzer, results []refreshResult) string {
	return renderDashboard(cfg, analyzers, results, nil)
}

func renderDashboard(cfg config, analyzers []*analyzer, results []refreshResult, history []sample) string {
	var out strings.Builder
	now := time.Now()
	groups := groupAnalyzers(analyzers)
	readyPods, readyContainers, totalContainers, restarts := 0, 0, 0, 0
	totalCPU, totalRAM := 0.0, 0.0
	metricPods := 0
	var summaryPods []pod
	for _, result := range results {
		if result.err != nil {
			continue
		}
		summaryPods = append(summaryPods, result.snapshot.Pod)
		sample := result.snapshot.Sample
		readyContainers += sample.Ready
		totalContainers += sample.Total
		restarts += sample.Restarts
		if podSnapshotReady(result.snapshot) {
			readyPods++
		}
		if sample.CPU >= 0 && sample.RAM >= 0 {
			totalCPU += sample.CPU
			totalRAM += sample.RAM
			metricPods++
		}
	}

	podNoun := "pods"
	readyPodNoun := "pods"
	if len(analyzers) == 1 {
		podNoun = "pod"
		readyPodNoun = "pod"
	}
	header := fmt.Sprintf(
		"KUBERNETES POD ANALYTICS · READ ONLY  %s / %s · %d %s · updated %s · every %s",
		strings.ToUpper(cfg.environment), cfg.namespace, len(analyzers), podNoun, now.Format("15:04:05"), cfg.refresh,
	)
	fmt.Fprintf(&out, "%s%s%s%s\n", color(cfg, colorBold), color(cfg, colorMagenta), header, color(cfg, colorReset))
	fmt.Fprintf(&out, "%sSUMMARY%s  %d/%d %s ready · %d/%d containers ready · %d restarts · %s\n",
		color(cfg, colorCyan), color(cfg, colorReset), readyPods, len(analyzers),
		readyPodNoun, readyContainers, totalContainers, restarts, resourceUsageSummary(
			cfg, totalCPU, totalRAM, resourcesForPods(summaryPods), metricPods == len(summaryPods) && len(summaryPods) > 0,
		),
	)
	help := "Ctrl-C to exit"
	if cfg.processes {
		help += " · process snapshots every " + cfg.processRefresh.String()
	}
	fmt.Fprintf(&out, "%s%s%s\n", color(cfg, colorDim), help, color(cfg, colorReset))
	fmt.Fprintf(&out, "%sCPU/REQ and RAM/REQ = usage/request · CPU/LIM and RAM/LIM = usage/limit%s\n", color(cfg, colorDim), color(cfg, colorReset))

	for _, group := range groups {
		groupReady, groupCPU, groupRAM, groupMetricPods := 0, 0.0, 0.0, 0
		groupState := "READY"
		groupPods := make([]pod, 0, len(group.indices))
		for _, index := range group.indices {
			result := results[index]
			state := podSnapshotState(result.snapshot, result.err)
			if stateRank(state) > stateRank(groupState) {
				groupState = state
			}
			if result.err != nil {
				continue
			}
			groupPods = append(groupPods, result.snapshot.Pod)
			if podSnapshotReady(result.snapshot) {
				groupReady++
			}
			if result.snapshot.Sample.CPU >= 0 && result.snapshot.Sample.RAM >= 0 {
				groupCPU += result.snapshot.Sample.CPU
				groupRAM += result.snapshot.Sample.RAM
				groupMetricPods++
			}
		}

		out.WriteString("\n")
		groupPodNoun := "pods"
		if len(group.indices) == 1 {
			groupPodNoun = "pod"
		}
		fmt.Fprintf(&out, "%s%s%s  %s%s%s · %d/%d %s ready · %s\n",
			color(cfg, colorBold), color(cfg, colorMagenta), group.series,
			color(cfg, stateColor(groupState)), groupState, color(cfg, colorReset),
			groupReady, len(group.indices), groupPodNoun, resourceUsageSummary(
				cfg, groupCPU, groupRAM, resourcesForPods(groupPods),
				groupMetricPods == len(groupPods) && len(groupPods) > 0,
			),
		)
		fmt.Fprintf(&out, "%-40s %-9s %5s %3s │ %8s %7s %7s │ %9s %7s %7s │ %-16s %s\n",
			"POD", "STATE", "READY", "RST", "CPU NOW", "CPU/REQ", "CPU/LIM", "RAM NOW", "RAM/REQ", "RAM/LIM", "NODE", "AGE")
		for _, index := range group.indices {
			result := results[index]
			podName := analyzers[index].cfg.pod
			if result.err != nil {
				writeMultiResourceRow(&out, cfg, truncate(podName, 40), "ERROR", "-", "-", -1, -1,
					resourceTotals{}, "-", "-")
				continue
			}
			snap := result.snapshot
			state := podSnapshotState(snap, nil)
			ready := fmt.Sprintf("%d/%d", snap.Sample.Ready, snap.Sample.Total)
			writeMultiResourceRow(&out, cfg, snap.Pod.Metadata.Name, state, ready, strconv.Itoa(snap.Sample.Restarts),
				snap.Sample.CPU, snap.Sample.RAM, resourcesForPods([]pod{snap.Pod}),
				dash(snap.Pod.Spec.NodeName), shortDuration(now.Sub(snap.Pod.Metadata.CreationTimestamp)))
			if len(snap.Pod.Spec.Containers) == 1 {
				continue
			}
			statuses := statusByName(snap.Pod.Status.ContainerStatuses)
			for _, spec := range snap.Pod.Spec.Containers {
				status := statuses[spec.Name]
				metric, hasMetric := snap.Metrics[spec.Name]
				containerState := "READY"
				if !status.Ready {
					containerState = "NOT READY"
				}
				cpu, ram := -1.0, -1.0
				if hasMetric {
					cpu, ram = metric.CPU, metric.RAM
				}
				writeMultiResourceRow(&out, cfg, "  ↳ "+spec.Name, containerState, fmt.Sprintf("%d/1", boolInt(status.Ready)),
					strconv.Itoa(status.RestartCount), cpu, ram, resourcesForContainers([]containerSpec{spec}), "", "")
			}
		}
	}

	var issues, warningEvents, processes, singleHistory strings.Builder
	renderMultiIssues(&issues, cfg, analyzers, results)
	renderMultiWarningEvents(&warningEvents, cfg, results, now)
	if cfg.processes {
		renderMultiProcesses(&processes, cfg, analyzers, results)
	}
	if len(analyzers) == 1 {
		renderSingleHistory(&singleHistory, cfg, history, now)
	}
	footer := fmt.Sprintf("\n%sCoverage: Kubernetes status/events and metrics-server CPU/RAM are observational. Process rows are best-effort read-only /proc samples through kubectl exec; unsupported images are collapsed into one notice.%s\n",
		color(cfg, colorDim), color(cfg, colorReset))
	return composeDashboard(
		cfg,
		out.String(),
		issues.String(),
		warningEvents.String(),
		processes.String(),
		singleHistory.String(),
		footer,
	)
}

type dashboardSection struct {
	content  string
	budget   int
	target   int
	minimum  int
	priority int
}

func composeDashboard(cfg config, core, issues, events, processes, history, footer string) string {
	full := core + issues + events + processes + history + footer
	if cfg.terminalLines <= 0 {
		return full
	}

	// Leave the final terminal row unused: printing a newline there scrolls the
	// alternate screen and makes the summary table disappear above the viewport.
	limit := max(1, cfg.terminalLines-1)
	if dashboardLineCount(full) <= limit {
		return full
	}
	if coreLines := dashboardLineCount(core); coreLines >= limit {
		return compactDashboardSection(core, limit)
	}

	sections := []dashboardSection{
		{content: issues, minimum: 4, priority: 1},
		{content: events, minimum: 4, priority: 2},
		{content: processes, minimum: 5, priority: 0},
		{content: history, minimum: 4, priority: 3},
	}
	sectionCaps := []int{8, 6, 8, 5}
	remaining := limit - dashboardLineCount(core)
	for index := range sections {
		lines := dashboardLineCount(sections[index].content)
		sections[index].target = min(lines, sectionCaps[index])
		sections[index].minimum = min(sections[index].minimum, sections[index].target)
	}

	for priority := 0; priority < len(sections); priority++ {
		for index := range sections {
			section := &sections[index]
			if section.priority != priority || section.minimum == 0 || remaining < section.minimum {
				continue
			}
			section.budget = section.minimum
			remaining -= section.minimum
		}
	}
	for priority := 0; priority < len(sections) && remaining > 0; priority++ {
		for index := range sections {
			section := &sections[index]
			if section.priority != priority || section.budget == 0 {
				continue
			}
			additional := min(section.target-section.budget, remaining)
			section.budget += additional
			remaining -= additional
		}
	}

	var out strings.Builder
	out.WriteString(core)
	for _, section := range sections {
		out.WriteString(compactDashboardSection(section.content, section.budget))
	}
	if dashboardLineCount(footer) <= remaining {
		out.WriteString(footer)
	}
	return out.String()
}

func dashboardLineCount(value string) int {
	trimmed := strings.TrimSuffix(value, "\n")
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

func compactDashboardSection(value string, budget int) string {
	lineCount := dashboardLineCount(value)
	if budget <= 0 || lineCount == 0 {
		return ""
	}
	if lineCount <= budget {
		return value
	}

	lines := strings.Split(strings.TrimSuffix(value, "\n"), "\n")
	if budget == 1 {
		return lines[0] + "\n"
	}
	visible := max(1, budget-1)
	hidden := len(lines) - visible
	return strings.Join(lines[:visible], "\n") + fmt.Sprintf("\n  … %d more rows hidden to fit the terminal\n", hidden)
}

func groupAnalyzers(analyzers []*analyzer) []multiGroup {
	groups := make([]multiGroup, 0, len(analyzers))
	positions := make(map[string]int, len(analyzers))
	for index, item := range analyzers {
		position, found := positions[item.series]
		if !found {
			position = len(groups)
			positions[item.series] = position
			groups = append(groups, multiGroup{series: item.series})
		}
		groups[position].indices = append(groups[position].indices, index)
	}
	return groups
}

func podSnapshotReady(snap snapshot) bool {
	return snap.Pod.Status.Phase == "Running" && snap.Sample.Total > 0 && snap.Sample.Ready == snap.Sample.Total
}

func podSnapshotState(snap snapshot, err error) string {
	if err != nil {
		return "ERROR"
	}
	switch snap.Pod.Status.Phase {
	case "Running":
		if podSnapshotReady(snap) {
			return "READY"
		}
		return "NOT READY"
	case "Succeeded":
		return "COMPLETE"
	case "Pending":
		return "PENDING"
	case "Failed":
		return "FAILED"
	case "":
		return "UNKNOWN"
	default:
		return strings.ToUpper(snap.Pod.Status.Phase)
	}
}

func stateRank(state string) int {
	switch state {
	case "ERROR", "FAILED", "NOT READY", "UNKNOWN":
		return 2
	case "WARN", "PENDING":
		return 1
	default:
		return 0
	}
}

func stateColor(state string) string {
	switch state {
	case "ERROR", "FAILED", "NOT READY", "UNKNOWN":
		return colorRed
	case "WARN", "PENDING":
		return colorYellow
	default:
		return colorGreen
	}
}

func formatSampleCPU(item sample) string {
	if item.CPU < 0 {
		return "-"
	}
	return formatCPU(item.CPU)
}

func formatSampleRAM(item sample) string {
	if item.RAM < 0 {
		return "-"
	}
	return formatBytes(item.RAM)
}

func resourceUsageSummary(cfg config, cpu, ram float64, totals resourceTotals, metricsComplete bool) string {
	if !metricsComplete {
		cpu, ram = -1, -1
	}
	return fmt.Sprintf("CPU %s (%s / %s) · RAM %s (%s / %s)",
		formatResourceUsage(cpu, true),
		utilizationLabel(cfg, cpu, totals.CPURequest, totals.CPURequestComplete, "req", false),
		utilizationLabel(cfg, cpu, totals.CPULimit, totals.CPULimitComplete, "lim", true),
		formatResourceUsage(ram, false),
		utilizationLabel(cfg, ram, totals.RAMRequest, totals.RAMRequestComplete, "req", false),
		utilizationLabel(cfg, ram, totals.RAMLimit, totals.RAMLimitComplete, "lim", true),
	)
}

func writeMultiResourceRow(out *strings.Builder, cfg config, name, state, ready, restarts string, cpu, ram float64, totals resourceTotals, node, age string) {
	fmt.Fprintf(out, "%-40s %s%-9s%s %5s %3s │ %8s %s %s │ %9s %s %s │ %-16s %s\n",
		truncate(name, 40), color(cfg, stateColor(state)), truncate(state, 9), color(cfg, colorReset),
		ready, restarts, formatResourceUsage(cpu, true),
		utilizationCell(cfg, cpu, totals.CPURequest, totals.CPURequestComplete, false, 7),
		utilizationCell(cfg, cpu, totals.CPULimit, totals.CPULimitComplete, true, 7),
		formatResourceUsage(ram, false),
		utilizationCell(cfg, ram, totals.RAMRequest, totals.RAMRequestComplete, false, 7),
		utilizationCell(cfg, ram, totals.RAMLimit, totals.RAMLimitComplete, true, 7),
		truncate(node, 16), age,
	)
}

func formatResourceUsage(value float64, cpu bool) string {
	if value < 0 {
		return "-"
	}
	if cpu {
		return formatCPU(value)
	}
	return formatBytes(value)
}

func utilizationPercent(used, total float64, complete bool) (int, bool) {
	if used < 0 || !complete || total <= 0 {
		return 0, false
	}
	return int(math.Round(used / total * 100)), true
}

func utilizationLabel(cfg config, used, total float64, complete bool, label string, limit bool) string {
	percent, ok := utilizationPercent(used, total, complete)
	text := "- " + label
	tint := colorDim
	if ok {
		text = fmt.Sprintf("%d%% %s", percent, label)
		tint = utilizationColor(percent, limit)
	}
	return color(cfg, tint) + text + color(cfg, colorReset)
}

func utilizationCell(cfg config, used, total float64, complete, limit bool, width int) string {
	percent, ok := utilizationPercent(used, total, complete)
	text := "-"
	tint := colorDim
	if ok {
		text = fmt.Sprintf("%d%%", percent)
		tint = utilizationColor(percent, limit)
	}
	return color(cfg, tint) + fmt.Sprintf("%*s", width, text) + color(cfg, colorReset)
}

func utilizationColor(percent int, limit bool) string {
	if limit && percent >= 90 {
		return colorRed
	}
	if percent > 100 || limit && percent >= 75 {
		return colorYellow
	}
	return colorGreen
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func renderMultiIssues(out *strings.Builder, cfg config, analyzers []*analyzer, results []refreshResult) {
	issues := make([]issueSummary, 0)
	positions := make(map[string]int)
	add := func(level, message string) {
		prefix, percent, suffix, percentage := splitPercentageFinding(message)
		keyMessage := message
		if percentage {
			keyMessage = prefix + "<percent>" + suffix
		}
		key := level + "\x00" + keyMessage
		if position, found := positions[key]; found {
			issues[position].count++
			if percentage {
				issues[position].minPercent = min(issues[position].minPercent, percent)
				issues[position].maxPercent = max(issues[position].maxPercent, percent)
			}
			return
		}
		positions[key] = len(issues)
		issues = append(issues, issueSummary{
			level: level, message: message, count: 1,
			percentage: percentage, prefix: prefix, suffix: suffix,
			minPercent: percent, maxPercent: percent,
		})
	}
	for index, result := range results {
		if result.err != nil {
			add("ERROR", fmt.Sprintf("pod/%s cannot be refreshed: %s", analyzers[index].cfg.pod, compactError(result.err.Error())))
			continue
		}
		for _, finding := range analyze(result.snapshot) {
			if (finding.Level == "WARN" || finding.Level == "ERROR") &&
				!strings.HasPrefix(finding.Message, "event ") {
				add(finding.Level, finding.Message)
			}
		}
	}
	if len(issues) == 0 {
		return
	}
	out.WriteString("\n")
	fmt.Fprintf(out, "%s%sISSUES%s\n", color(cfg, colorBold), color(cfg, colorCyan), color(cfg, colorReset))
	for _, issue := range issues[:min(12, len(issues))] {
		message := issue.message
		if issue.percentage {
			value := fmt.Sprintf("%d%%", issue.minPercent)
			if issue.minPercent != issue.maxPercent {
				value = fmt.Sprintf("%d–%d%%", issue.minPercent, issue.maxPercent)
			}
			message = issue.prefix + value + issue.suffix
		}
		count := ""
		if issue.count > 1 {
			count = fmt.Sprintf(" x%d", issue.count)
		}
		fmt.Fprintf(out, "%s%-5s%s%s  %s\n", color(cfg, stateColor(issue.level)), issue.level, count, color(cfg, colorReset), message)
	}
}

func renderMultiWarningEvents(out *strings.Builder, cfg config, results []refreshResult, now time.Time) {
	const window = 10 * time.Minute
	var summaries []warningEventSummary
	positions := make(map[string]int)
	for _, result := range results {
		if result.err != nil {
			continue
		}
		for _, item := range result.snapshot.Events {
			latest := eventTime(item)
			if item.Type != "Warning" || latest.Before(now.Add(-window)) {
				continue
			}
			message := compactEventMessage(item.Message)
			key := item.Reason + "\x00" + message
			if position, found := positions[key]; found {
				summaries[position].count += eventCount(item)
				if latest.After(summaries[position].latest) {
					summaries[position].latest = latest
				}
				continue
			}
			positions[key] = len(summaries)
			summaries = append(summaries, warningEventSummary{
				reason: item.Reason, message: message, count: eventCount(item), latest: latest,
			})
		}
	}
	if len(summaries) == 0 {
		return
	}
	sort.SliceStable(summaries, func(i, j int) bool { return summaries[i].latest.After(summaries[j].latest) })
	out.WriteString("\n")
	fmt.Fprintf(out, "%s%sWARNING EVENTS · last %s%s\n", color(cfg, colorBold), color(cfg, colorCyan), window, color(cfg, colorReset))
	for _, item := range summaries[:min(6, len(summaries))] {
		fmt.Fprintf(out, "%sWARN%s  %-18s %5s ago  x%-3d %s\n",
			color(cfg, colorYellow), color(cfg, colorReset), truncate(item.reason, 18),
			shortDuration(now.Sub(item.latest)), item.count, truncate(item.message, 90),
		)
	}
}

func compactEventMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	for _, scheme := range []string{"http://", "https://"} {
		start := strings.Index(message, scheme)
		if start < 0 {
			continue
		}
		hostStart := start + len(scheme)
		pathOffset := strings.Index(message[hostStart:], "/")
		if pathOffset < 0 {
			return message[:hostStart] + "<pod>"
		}
		pathStart := hostStart + pathOffset
		return message[:hostStart] + "<pod>" + message[pathStart:]
	}
	return message
}

func splitPercentageFinding(message string) (string, int, string, bool) {
	percentEnd := strings.Index(message, "% of ")
	if percentEnd < 0 {
		return "", 0, "", false
	}
	percentStart := strings.LastIndex(message[:percentEnd], " ") + 1
	if percentStart <= 0 || percentStart >= percentEnd {
		return "", 0, "", false
	}
	value, err := strconv.Atoi(message[percentStart:percentEnd])
	if err != nil {
		return "", 0, "", false
	}
	return message[:percentStart], value, message[percentEnd+1:], true
}

func renderMultiProcesses(out *strings.Builder, cfg config, analyzers []*analyzer, results []refreshResult) {
	var attributions []processAttribution
	unavailable := make(map[string]int)
	for index, result := range results {
		if result.err != nil {
			continue
		}
		for container, rows := range result.snapshot.Processes {
			if len(rows) == 0 {
				continue
			}
			item := processAttribution{
				pod: analyzers[index].cfg.pod, container: container,
				processCount: len(rows), hottest: rows[0],
			}
			for _, process := range rows {
				item.totalRSSKiB += process.RSSKiB
				if process.CPU >= 0 {
					item.totalCPU += process.CPU
					item.hasCPU = true
				}
			}
			item.containerUse, item.hasUsage = result.snapshot.Metrics[container]
			attributions = append(attributions, item)
		}
		for _, message := range result.snapshot.ProcessErrors {
			unavailable[compactProcessError(message)]++
		}
	}
	if len(attributions) == 0 && len(unavailable) == 0 {
		return
	}
	sort.SliceStable(attributions, func(i, j int) bool {
		if attributions[i].hasCPU != attributions[j].hasCPU {
			return attributions[i].hasCPU
		}
		if attributions[i].totalCPU == attributions[j].totalCPU {
			return attributions[i].totalRSSKiB > attributions[j].totalRSSKiB
		}
		return attributions[i].totalCPU > attributions[j].totalCPU
	})
	out.WriteString("\n")
	title := "PROCESSES"
	if len(attributions) > 0 {
		title = "PROCESS ATTRIBUTION · /proc totals compared with container metrics"
	}
	fmt.Fprintf(out, "%s%s%s%s\n", color(cfg, colorBold), color(cfg, colorCyan), title, color(cfg, colorReset))
	if len(attributions) > 0 {
		out.WriteString("POD                              CONTAINER          PROCS  PROC CPU  CTR CPU  HOT CPU  PROC RSS*   CTR RAM  HOTTEST\n")
		for _, item := range attributions[:min(10, len(attributions))] {
			containerCPU, containerRAM := "-", "-"
			if item.hasUsage {
				containerCPU = formatCPU(item.containerUse.CPU)
				containerRAM = formatBytes(item.containerUse.RAM)
			}
			fmt.Fprintf(out, "%-32s %-18s %5d %9s %8s %8s %10s %9s  %s\n",
				truncate(item.pod, 32), truncate(item.container, 18), item.processCount,
				formatProcessCPU(item.totalCPU, item.hasCPU), containerCPU,
				formatProcessCPU(item.hottest.CPU, item.hottest.CPU >= 0),
				formatBytes(float64(item.totalRSSKiB)*1024), containerRAM,
				truncate(fmt.Sprintf("%s[%d]", item.hottest.Command, item.hottest.PID), 42),
			)
		}
		fmt.Fprintf(out, "%sREAD: PROC CPU/RSS = all visible processes; CTR CPU/RAM = metrics-server total; HOT CPU = busiest PID. *RSS may double-count shared pages and sampling windows may differ.%s\n",
			color(cfg, colorDim), color(cfg, colorReset))
	}
	messages := make([]string, 0, len(unavailable))
	for message := range unavailable {
		messages = append(messages, message)
	}
	sort.Strings(messages)
	for _, message := range messages {
		count := unavailable[message]
		unit := "container"
		if count != 1 {
			unit = "containers"
		}
		fmt.Fprintf(out, "%sWARN%s  %d %s: %s\n", color(cfg, colorYellow), color(cfg, colorReset), count, unit, message)
	}
}

func formatProcessCPU(percent float64, sampled bool) string {
	if !sampled || percent < 0 {
		return "sample"
	}
	return formatCPU(percent * 10)
}

func compactProcessError(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "exit code 126"), strings.Contains(lower, "exit code 127"), strings.Contains(lower, "sh: not found"):
		return "shell-based /proc collector is unavailable in the container image"
	case strings.Contains(lower, "/proc snapshot"), strings.Contains(lower, "/proc returned"):
		return "process data is unavailable through /proc"
	case strings.Contains(lower, "forbidden"):
		return "kubectl exec is not permitted by cluster policy"
	case strings.Contains(lower, "unable to upgrade connection"):
		return "kubectl exec transport is unavailable"
	default:
		return compactError(message)
	}
}

func formatProcessPercent(value float64, sampling bool) string {
	if value < 0 {
		if sampling {
			return "sample"
		}
		return "-"
	}
	return fmt.Sprintf("%.1f", value)
}

func compactError(message string) string {
	return truncate(strings.Join(strings.Fields(message), " "), 100)
}

func render(cfg config, series, historyPath string, history []sample, snap snapshot) string {
	_ = historyPath // History remains persisted, but the dashboard omits the host-specific absolute path.
	podName := snap.Pod.Metadata.Name
	item := &analyzer{
		cfg:    config{pod: podName},
		series: series,
	}
	return renderDashboard(cfg, []*analyzer{item}, []refreshResult{{snapshot: snap}}, history)
}

func renderSingleHistory(out *strings.Builder, cfg config, history []sample, now time.Time) {
	window := trimSamples(history, now.Add(-cfg.historyWindow))
	if len(window) == 0 || countMetricSamples(window) == 0 {
		return
	}
	cpuValues := metricValues(window, func(item sample) float64 { return item.CPU })
	ramValues := metricValues(window, func(item sample) float64 { return item.RAM })
	out.WriteString("\n")
	fmt.Fprintf(out, "%s%sHISTORY · local samples across pod replacements%s\n",
		color(cfg, colorBold), color(cfg, colorCyan), color(cfg, colorReset))
	fmt.Fprintf(out, "CPU  %s  NOW %s · AVG %s · P95 %s · MAX %s\n",
		sparkline(cpuValues, cfg.chartPoints), formatCPU(last(cpuValues)), formatCPU(average(cpuValues)),
		formatCPU(percentile(cpuValues, 0.95)), formatCPU(maximum(cpuValues)))
	fmt.Fprintf(out, "RAM  %s  NOW %s · AVG %s · P95 %s · MAX %s\n",
		sparkline(ramValues, cfg.chartPoints), formatBytes(last(ramValues)), formatBytes(average(ramValues)),
		formatBytes(percentile(ramValues, 0.95)), formatBytes(maximum(ramValues)))
	fmt.Fprintf(out, "%sWINDOW  %s → %s · %d samples · %d pod UID(s)%s\n",
		color(cfg, colorDim), window[0].At.Local().Format("Jan 02 15:04"),
		window[len(window)-1].At.Local().Format("Jan 02 15:04"), countMetricSamples(window),
		distinctUIDs(window), color(cfg, colorReset))
}

func analyze(snap snapshot) []finding {
	var findings []finding
	item := snap.Pod
	if item.Status.Phase != "Running" && item.Status.Phase != "Succeeded" {
		findings = append(findings, finding{"ERROR", "Pod phase is " + dash(item.Status.Phase) + formatReason(item.Status.Reason)})
	}
	statuses := statusByName(item.Status.ContainerStatuses)
	for _, spec := range item.Spec.Containers {
		status := statuses[spec.Name]
		if !status.Ready {
			findings = append(findings, finding{"ERROR", fmt.Sprintf("container/%s is not Ready (%s)", spec.Name, containerDisplayState(status))})
		}
		if status.RestartCount > 0 {
			findings = append(findings, finding{"WARN", fmt.Sprintf("container/%s restarted %d time(s)", spec.Name, status.RestartCount)})
		}
		if status.LastState.Terminated != nil {
			terminated := status.LastState.Terminated
			level := "WARN"
			if terminated.Reason == "OOMKilled" || terminated.ExitCode != 0 {
				level = "ERROR"
			}
			findings = append(findings, finding{level, fmt.Sprintf("container/%s last exit: %s (code %d)", spec.Name, dash(terminated.Reason), terminated.ExitCode)})
		}
		metric, hasMetric := snap.Metrics[spec.Name]
		if !hasMetric {
			continue
		}
		addUtilizationFindings(&findings, spec.Name, "CPU", metric.CPU, spec.Resources.Requests["cpu"], spec.Resources.Limits["cpu"], parseCPU)
		addUtilizationFindings(&findings, spec.Name, "memory", metric.RAM, spec.Resources.Requests["memory"], spec.Resources.Limits["memory"], parseBytes)
		if spec.Resources.Requests["cpu"] == "" || spec.Resources.Requests["memory"] == "" {
			findings = append(findings, finding{"INFO", fmt.Sprintf("container/%s has incomplete CPU/memory requests; scheduling and utilization baselines are weaker", spec.Name)})
		}
		if spec.Resources.Limits["memory"] == "" {
			findings = append(findings, finding{"INFO", fmt.Sprintf("container/%s has no memory limit; node pressure can still evict it", spec.Name)})
		}
	}
	for _, condition := range item.Status.Conditions {
		if condition.Type == "Ready" && condition.Status != "True" {
			findings = append(findings, finding{"ERROR", "Ready condition is " + condition.Status + formatReason(condition.Reason)})
		}
	}
	for _, item := range snap.Events {
		if item.Type == "Warning" {
			findings = append(findings, finding{"WARN", fmt.Sprintf("event %s x%d: %s", item.Reason, eventCount(item), truncate(strings.Join(strings.Fields(item.Message), " "), 90))})
			if len(findings) >= 12 {
				break
			}
		}
	}
	if snap.MetricsError != "" {
		findings = append(findings, finding{"WARN", "CPU/RAM analysis is incomplete because metrics-server data is unavailable"})
	}
	if len(findings) == 0 {
		findings = append(findings, finding{"OK", "No readiness, restart, OOM, event or resource-pressure warning detected in the available data"})
	}
	return findings
}

func addUtilizationFindings(findings *[]finding, container, resource string, used float64, request, limit string, parser func(string) (float64, error)) {
	if limit != "" {
		if parsed, err := parser(limit); err == nil && parsed > 0 && used/parsed >= 0.9 {
			*findings = append(*findings, finding{"ERROR", fmt.Sprintf("container/%s %s usage is %.0f%% of limit", container, resource, used/parsed*100)})
		}
	}
	if request != "" {
		if parsed, err := parser(request); err == nil && parsed > 0 && used/parsed > 1 {
			*findings = append(*findings, finding{"WARN", fmt.Sprintf("container/%s %s usage is %.0f%% of request", container, resource, used/parsed*100)})
		}
	}
}

func statusByName(statuses []containerStatus) map[string]containerStatus {
	result := make(map[string]containerStatus, len(statuses))
	for _, status := range statuses {
		result[status.Name] = status
	}
	return result
}

func containerDisplayState(status containerStatus) string {
	if status.State.Waiting != nil {
		return "Waiting:" + dash(status.State.Waiting.Reason)
	}
	if status.State.Terminated != nil {
		return fmt.Sprintf("Exited:%d", status.State.Terminated.ExitCode)
	}
	if status.State.Running != nil {
		if status.Ready {
			return "Running/Ready"
		}
		return "Running/NotReady"
	}
	return "Unknown"
}

func resourceTriple(used float64, hasMetric bool, request, limit string, cpu bool) string {
	usageText := "-"
	if hasMetric {
		if cpu {
			usageText = formatCPU(used)
		} else {
			usageText = formatBytes(used)
		}
	}
	return fmt.Sprintf("%s / %s / %s", usageText, dash(request), dash(limit))
}

func eventTime(item event) time.Time {
	if item.Series != nil && !item.Series.LastObservedTime.IsZero() {
		return item.Series.LastObservedTime
	}
	if !item.EventTime.IsZero() {
		return item.EventTime
	}
	if !item.LastTimestamp.IsZero() {
		return item.LastTimestamp
	}
	return item.Metadata.CreationTimestamp
}

func eventCount(item event) int {
	if item.Series != nil && item.Series.Count > 0 {
		return item.Series.Count
	}
	if item.Count > 0 {
		return item.Count
	}
	return 1
}

func metricValues(samples []sample, value func(sample) float64) []float64 {
	values := make([]float64, 0, len(samples))
	for _, item := range samples {
		if current := value(item); current >= 0 {
			values = append(values, current)
		}
	}
	return values
}

func countMetricSamples(samples []sample) int {
	count := 0
	for _, item := range samples {
		if item.CPU >= 0 && item.RAM >= 0 {
			count++
		}
	}
	return count
}

func distinctUIDs(samples []sample) int {
	uids := make(map[string]struct{})
	for _, item := range samples {
		uids[item.PodUID] = struct{}{}
	}
	return len(uids)
}

func sparkline(values []float64, points int) string {
	if len(values) == 0 {
		return "-"
	}
	values = downsample(values, points)
	minValue, maxValue := values[0], values[0]
	for _, value := range values[1:] {
		minValue = math.Min(minValue, value)
		maxValue = math.Max(maxValue, value)
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	var result strings.Builder
	for _, value := range values {
		index := 0
		if maxValue > minValue {
			index = int(math.Round((value - minValue) / (maxValue - minValue) * float64(len(blocks)-1)))
		}
		result.WriteRune(blocks[index])
	}
	return result.String()
}

func downsample(values []float64, points int) []float64 {
	if len(values) <= points {
		return values
	}
	result := make([]float64, 0, points)
	for index := 0; index < points; index++ {
		start := index * len(values) / points
		end := (index + 1) * len(values) / points
		result = append(result, average(values[start:end]))
	}
	return result
}

func percentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyOfValues := append([]float64(nil), values...)
	sort.Float64s(copyOfValues)
	index := int(math.Ceil(quantile*float64(len(copyOfValues)))) - 1
	index = max(0, min(index, len(copyOfValues)-1))
	return copyOfValues[index]
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func maximum(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		result = math.Max(result, value)
	}
	return result
}

func last(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

func formatCPU(millicores float64) string {
	if millicores >= 1000 {
		return fmt.Sprintf("%.2f cores", millicores/1000)
	}
	return fmt.Sprintf("%.0fm", millicores)
}

func formatBytes(bytes float64) string {
	units := []string{"B", "Ki", "Mi", "Gi", "Ti"}
	index := 0
	for bytes >= 1024 && index < len(units)-1 {
		bytes /= 1024
		index++
	}
	if index < 2 {
		return fmt.Sprintf("%.0f%s", bytes, units[index])
	}
	return fmt.Sprintf("%.1f%s", bytes, units[index])
}

func shortDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	if duration < 48*time.Hour {
		return fmt.Sprintf("%dh%02dm", int(duration.Hours()), int(duration.Minutes())%60)
	}
	return fmt.Sprintf("%dd%02dh", int(duration.Hours())/24, int(duration.Hours())%24)
}

func formatReason(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func color(cfg config, code string) string {
	if !cfg.color {
		return ""
	}
	return code
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func detectTerminalLines() int {
	if value, err := strconv.Atoi(os.Getenv("LINES")); err == nil && value > 0 {
		return value
	}
	if !isTerminal(os.Stdout) {
		return 0
	}
	command := exec.Command("stty", "size")
	command.Stdin = os.Stdin
	output, err := command.Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return 0
	}
	lines, err := strconv.Atoi(fields[0])
	if err != nil || lines <= 0 {
		return 0
	}
	return lines
}
