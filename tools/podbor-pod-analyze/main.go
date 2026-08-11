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
	"syscall"
	"time"
)

const (
	colorReset  = "\x1b[0m"
	colorDim    = "\x1b[2m"
	colorRed    = "\x1b[31m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
	colorCyan   = "\x1b[36m"
)

type config struct {
	kubectl       string
	kubeconfig    string
	context       string
	namespace     string
	environment   string
	pod           string
	historyDir    string
	refresh       time.Duration
	historyWindow time.Duration
	retention     time.Duration
	chartPoints   int
	once          bool
	color         bool
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
	Pod          pod
	Metrics      map[string]usage
	MetricsError string
	Events       []event
	Sample       sample
}

type finding struct {
	Level   string
	Message string
}

type analyzer struct {
	cfg         config
	series      string
	historyPath string
	history     []sample
	events      []event
	eventsAt    time.Time
	lastError   string
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	initial, err := fetchPod(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read pod/%s: %v\n", cfg.pod, err)
		os.Exit(1)
	}
	series := resolveSeries(ctx, cfg, initial)
	historyPath := historyFile(cfg, series)
	history, err := loadHistory(historyPath, time.Now().Add(-cfg.retention))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot load metrics history: %v\n", err)
		os.Exit(1)
	}
	if err := rewriteHistory(historyPath, history); err != nil {
		fmt.Fprintf(os.Stderr, "cannot prune metrics history: %v\n", err)
		os.Exit(1)
	}

	a := &analyzer{
		cfg:         cfg,
		series:      series,
		historyPath: historyPath,
		history:     history,
	}

	if cfg.once || !isTerminal(os.Stdout) {
		if err := a.refresh(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	ticker := time.NewTicker(cfg.refresh)
	defer ticker.Stop()
	for {
		_ = a.refresh(ctx)
		select {
		case <-ctx.Done():
			fmt.Println()
			return
		case <-ticker.C:
		}
	}
}

func parseConfig() (config, error) {
	home, _ := os.UserHomeDir()
	cfg := config{}
	flag.StringVar(&cfg.kubectl, "kubectl", "kubectl", "kubectl executable")
	flag.StringVar(&cfg.kubeconfig, "kubeconfig", "", "pinned kubeconfig")
	flag.StringVar(&cfg.context, "context", "", "pinned context")
	flag.StringVar(&cfg.namespace, "namespace", "", "namespace")
	flag.StringVar(&cfg.environment, "environment", "", "environment label")
	flag.StringVar(&cfg.pod, "pod", "", "pod name")
	flag.StringVar(&cfg.historyDir, "history-dir", filepath.Join(home, ".local", "state", "podbor-kube", "pod-metrics"), "local metric history directory")
	flag.DurationVar(&cfg.refresh, "refresh", 5*time.Second, "live refresh interval")
	flag.DurationVar(&cfg.historyWindow, "history-window", 24*time.Hour, "history shown in charts")
	flag.DurationVar(&cfg.retention, "retention", 30*24*time.Hour, "local history retention")
	flag.IntVar(&cfg.chartPoints, "chart-points", 72, "maximum terminal chart points")
	flag.BoolVar(&cfg.once, "once", false, "render one snapshot and exit")
	flag.Parse()
	cfg.color = os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout)

	var missing []string
	for name, value := range map[string]string{
		"--kubeconfig":  cfg.kubeconfig,
		"--context":     cfg.context,
		"--namespace":   cfg.namespace,
		"--environment": cfg.environment,
		"--pod":         cfg.pod,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return cfg, fmt.Errorf("required flags are missing: %s", strings.Join(missing, ", "))
	}
	if cfg.refresh < time.Second {
		return cfg, errors.New("--refresh must be at least 1s")
	}
	if cfg.historyWindow <= 0 || cfg.retention <= 0 || cfg.retention < cfg.historyWindow {
		return cfg, errors.New("history window and retention must be positive; retention must cover the window")
	}
	if cfg.chartPoints < 8 {
		return cfg, errors.New("--chart-points must be at least 8")
	}
	return cfg, nil
}

func (a *analyzer) refresh(ctx context.Context) error {
	item, err := fetchPod(ctx, a.cfg)
	if err != nil {
		a.lastError = err.Error()
		if a.cfg.color {
			fmt.Print("\x1b[2J\x1b[H")
		}
		fmt.Printf("Podbor %s · %s · pod/%s · READ ONLY\n\n", a.cfg.environment, a.cfg.namespace, a.cfg.pod)
		fmt.Printf("ERROR: cannot refresh pod: %v\n", err)
		return err
	}

	metrics, metricsErr := fetchMetrics(ctx, a.cfg)
	if time.Since(a.eventsAt) >= 30*time.Second || a.eventsAt.IsZero() {
		if events, eventErr := fetchEvents(ctx, a.cfg); eventErr == nil {
			a.events = events
			a.eventsAt = time.Now()
		}
	}

	s := makeSample(item, metrics)
	if err := appendHistory(a.historyPath, s); err != nil {
		a.lastError = "history: " + err.Error()
	} else {
		a.history = append(a.history, s)
	}
	a.history = trimSamples(a.history, time.Now().Add(-a.cfg.retention))

	snap := snapshot{Pod: item, Metrics: metrics, Events: a.events, Sample: s}
	if metricsErr != nil {
		snap.MetricsError = metricsErr.Error()
	}
	if a.cfg.color {
		fmt.Print("\x1b[2J\x1b[H")
	}
	fmt.Print(render(a.cfg, a.series, a.historyPath, a.history, snap))
	return nil
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

func render(cfg config, series, historyPath string, history []sample, snap snapshot) string {
	var out strings.Builder
	item := snap.Pod
	now := time.Now()
	window := trimSamples(history, now.Add(-cfg.historyWindow))
	ready := 0
	restarts := 0
	for _, status := range item.Status.ContainerStatuses {
		restarts += status.RestartCount
		if status.Ready {
			ready++
		}
	}

	fmt.Fprintf(&out, "%sPodbor %s · %s · pod/%s · READ ONLY%s\n", color(cfg, colorCyan), cfg.environment, cfg.namespace, item.Metadata.Name, color(cfg, colorReset))
	fmt.Fprintf(&out, "Workload: %-36s Phase: %-10s Ready: %d/%d  Restarts: %d\n", series, item.Status.Phase, ready, len(item.Status.ContainerStatuses), restarts)
	fmt.Fprintf(&out, "Node: %-24s Pod IP: %-16s QoS: %-10s Age: %s\n", dash(item.Spec.NodeName), dash(item.Status.PodIP), dash(item.Status.QOSClass), shortDuration(now.Sub(item.Metadata.CreationTimestamp)))
	fmt.Fprintf(&out, "Updated: %s · refresh %s · Ctrl-C to exit\n\n", now.Format("2006-01-02 15:04:05"), cfg.refresh)

	out.WriteString("CONTAINERS · usage / request / limit\n")
	out.WriteString("NAME                     STATE              RESTARTS  CPU                       MEMORY\n")
	statuses := statusByName(item.Status.ContainerStatuses)
	for _, spec := range item.Spec.Containers {
		status := statuses[spec.Name]
		metric, hasMetric := snap.Metrics[spec.Name]
		state := containerDisplayState(status)
		cpu := resourceTriple(metric.CPU, hasMetric, spec.Resources.Requests["cpu"], spec.Resources.Limits["cpu"], true)
		ram := resourceTriple(metric.RAM, hasMetric, spec.Resources.Requests["memory"], spec.Resources.Limits["memory"], false)
		fmt.Fprintf(&out, "%-24s %-18s %8d  %-25s %s\n", truncate(spec.Name, 24), truncate(state, 18), status.RestartCount, cpu, ram)
	}
	if snap.MetricsError != "" {
		fmt.Fprintf(&out, "%sMetrics unavailable: %s%s\n", color(cfg, colorYellow), truncate(snap.MetricsError, 110), color(cfg, colorReset))
	}

	out.WriteString("\nHISTORY · locally sampled across pod replacements\n")
	if len(window) == 0 || countMetricSamples(window) == 0 {
		out.WriteString("No resource samples yet. Keep this screen open; history is retained for later runs.\n")
	} else {
		cpuValues := metricValues(window, func(item sample) float64 { return item.CPU })
		ramValues := metricValues(window, func(item sample) float64 { return item.RAM })
		fmt.Fprintf(&out, "CPU  %s  now %s · avg %s · p95 %s · max %s\n", sparkline(cpuValues, cfg.chartPoints), formatCPU(last(cpuValues)), formatCPU(average(cpuValues)), formatCPU(percentile(cpuValues, 0.95)), formatCPU(maximum(cpuValues)))
		fmt.Fprintf(&out, "RAM  %s  now %s · avg %s · p95 %s · max %s\n", sparkline(ramValues, cfg.chartPoints), formatBytes(last(ramValues)), formatBytes(average(ramValues)), formatBytes(percentile(ramValues, 0.95)), formatBytes(maximum(ramValues)))
		fmt.Fprintf(&out, "Range %s → %s · %d samples · %d pod UID(s)\n", window[0].At.Local().Format("Jan 02 15:04"), window[len(window)-1].At.Local().Format("Jan 02 15:04"), countMetricSamples(window), distinctUIDs(window))
	}
	fmt.Fprintf(&out, "%sCSV: %s%s\n", color(cfg, colorDim), historyPath, color(cfg, colorReset))

	out.WriteString("\nANALYSIS\n")
	findings := analyze(snap)
	for _, item := range findings {
		marker, tint := "INFO", colorCyan
		switch item.Level {
		case "WARN":
			marker, tint = "WARN", colorYellow
		case "ERROR":
			marker, tint = "ERROR", colorRed
		case "OK":
			marker, tint = "OK", colorGreen
		}
		fmt.Fprintf(&out, "%s%-5s%s %s\n", color(cfg, tint), marker, color(cfg, colorReset), item.Message)
	}

	out.WriteString("\nRECENT EVENTS\n")
	if len(snap.Events) == 0 {
		out.WriteString("No retained events for this pod.\n")
	} else {
		limit := min(6, len(snap.Events))
		for _, item := range snap.Events[:limit] {
			fmt.Fprintf(&out, "%-7s %-20s %5s ago  x%-3d %s\n", item.Type, truncate(item.Reason, 20), shortDuration(now.Sub(eventTime(item))), eventCount(item), truncate(strings.Join(strings.Fields(item.Message), " "), 72))
		}
	}

	fmt.Fprintf(&out, "\n%sCoverage: metrics-server exposes CPU/RAM snapshots only. Local CSV supplies history; CPU throttling, network, disk I/O and history before first run require Prometheus/cAdvisor (or another metrics backend).%s\n", color(cfg, colorDim), color(cfg, colorReset))
	return out.String()
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
