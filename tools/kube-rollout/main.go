package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	defaultRefresh = 100 * time.Millisecond

	colorReset   = "\x1b[0m"
	colorDim     = "\x1b[2m"
	colorRed     = "\x1b[31m"
	colorGreen   = "\x1b[32m"
	colorYellow  = "\x1b[33m"
	colorBlue    = "\x1b[34m"
	colorMagenta = "\x1b[35m"
	colorCyan    = "\x1b[36m"
)

type config struct {
	kubectl     string
	kubeconfig  string
	context     string
	namespace   string
	environment string
	resources   stringList
	refresh     time.Duration
	tail        int
}

type stringList []string

func (items *stringList) String() string {
	return strings.Join(*items, ",")
}

func (items *stringList) Set(value string) error {
	if value == "" || !strings.Contains(value, "/") {
		return errors.New("resource must look like deployment/name")
	}
	*items = append(*items, value)
	return nil
}

type metadata struct {
	Name              string            `json:"name"`
	Generation        int64             `json:"generation"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	DeletionTimestamp *time.Time        `json:"deletionTimestamp"`
	Labels            map[string]string `json:"labels"`
}

type condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type labelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

type workload struct {
	Kind     string   `json:"kind"`
	Metadata metadata `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
		Selector struct {
			MatchLabels      map[string]string          `json:"matchLabels"`
			MatchExpressions []labelSelectorRequirement `json:"matchExpressions"`
		} `json:"selector"`
	} `json:"spec"`
	Status struct {
		ObservedGeneration     int64       `json:"observedGeneration"`
		Replicas               int         `json:"replicas"`
		UpdatedReplicas        int         `json:"updatedReplicas"`
		ReadyReplicas          int         `json:"readyReplicas"`
		AvailableReplicas      int         `json:"availableReplicas"`
		UnavailableReplicas    int         `json:"unavailableReplicas"`
		CurrentReplicas        int         `json:"currentReplicas"`
		CurrentRevision        string      `json:"currentRevision"`
		UpdateRevision         string      `json:"updateRevision"`
		DesiredNumberScheduled int         `json:"desiredNumberScheduled"`
		UpdatedNumberScheduled int         `json:"updatedNumberScheduled"`
		NumberReady            int         `json:"numberReady"`
		NumberAvailable        int         `json:"numberAvailable"`
		NumberUnavailable      int         `json:"numberUnavailable"`
		Conditions             []condition `json:"conditions"`
		CollisionCount         *int        `json:"collisionCount"`
		NumberMisscheduled     int         `json:"numberMisscheduled"`
		CurrentNumberScheduled int         `json:"currentNumberScheduled"`
	} `json:"status"`
}

type workloadCollection struct {
	Items []workload `json:"items"`
}

func (items *workloadCollection) UnmarshalJSON(data []byte) error {
	var envelope struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if strings.HasSuffix(envelope.Kind, "List") || envelope.Kind == "List" {
		type collection workloadCollection
		return json.Unmarshal(data, (*collection)(items))
	}

	var item workload
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}
	items.Items = []workload{item}
	return nil
}

type containerState struct {
	Waiting *struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	} `json:"waiting"`
	Running *struct {
		StartedAt time.Time `json:"startedAt"`
	} `json:"running"`
	Terminated *struct {
		Reason   string `json:"reason"`
		Message  string `json:"message"`
		ExitCode int    `json:"exitCode"`
	} `json:"terminated"`
}

type containerStatus struct {
	Name         string         `json:"name"`
	Ready        bool           `json:"ready"`
	RestartCount int            `json:"restartCount"`
	State        containerState `json:"state"`
}

type pod struct {
	Metadata metadata `json:"metadata"`
	Status   struct {
		Phase             string            `json:"phase"`
		Reason            string            `json:"reason"`
		Message           string            `json:"message"`
		PodIP             string            `json:"podIP"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type podList struct {
	Items []pod `json:"items"`
}

type rolloutSummary struct {
	State       string
	Desired     int
	Updated     int
	Ready       int
	Available   int
	Unavailable int
	Message     string
}

type workloadView struct {
	workload workload
	summary  rolloutSummary
}

type logLine struct {
	pod  string
	text string
	at   time.Time
}

type logStream struct {
	cancel context.CancelFunc
}

type dashboard struct {
	cfg          config
	workloads    []workloadView
	pods         []pod
	summary      rolloutSummary
	lastError    string
	lastRefresh  time.Time
	lifecycle    []string
	logs         []logLine
	previousPods map[string]string
	streams      map[string]logStream
	logLines     chan logLine
	streamDone   chan string
	width        int
	height       int
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := &dashboard{
		cfg:          cfg,
		previousPods: make(map[string]string),
		streams:      make(map[string]logStream),
		logLines:     make(chan logLine, 256),
		streamDone:   make(chan string, 32),
		width:        120,
		height:       36,
	}
	d.width, d.height = terminalSize()

	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)

	fmt.Print("\x1b[?1049h\x1b[?25l")
	defer fmt.Print("\x1b[?25h\x1b[?1049l")

	if err := d.poll(ctx); err != nil {
		d.lastError = err.Error()
	}
	d.render()

	pollTicker := time.NewTicker(cfg.refresh)
	renderRefresh := cfg.refresh
	if renderRefresh > 200*time.Millisecond {
		renderRefresh = 200 * time.Millisecond
	}
	renderTicker := time.NewTicker(renderRefresh)
	defer pollTicker.Stop()
	defer renderTicker.Stop()

	dirty := false
	for {
		select {
		case <-ctx.Done():
			d.stopStreams()
			return
		case <-pollTicker.C:
			err := d.poll(ctx)
			if ctx.Err() != nil {
				d.stopStreams()
				return
			}
			if err != nil {
				d.lastError = err.Error()
			} else {
				d.lastError = ""
			}
			dirty = true
		case line := <-d.logLines:
			d.logs = appendBounded(d.logs, line, 1000)
			dirty = true
		case podName := <-d.streamDone:
			delete(d.streams, podName)
		case <-resize:
			d.width, d.height = terminalSize()
			dirty = true
		case <-renderTicker.C:
			if dirty {
				d.render()
				dirty = false
			}
		}
	}
}

func parseConfig() (config, error) {
	var cfg config
	flag.StringVar(&cfg.kubectl, "kubectl", "kubectl", "path to kubectl")
	flag.StringVar(&cfg.kubeconfig, "kubeconfig", "", "kubeconfig path")
	flag.StringVar(&cfg.context, "context", "", "Kubernetes context")
	flag.StringVar(&cfg.namespace, "namespace", "", "Kubernetes namespace")
	flag.StringVar(&cfg.environment, "environment", "", "short environment label")
	flag.Var(&cfg.resources, "resource", "workload resource/name (repeatable)")
	flag.DurationVar(&cfg.refresh, "refresh", defaultRefresh, "refresh interval")
	flag.IntVar(&cfg.tail, "tail", 20, "initial log lines per pod")
	flag.Parse()

	switch {
	case cfg.kubeconfig == "":
		return cfg, errors.New("--kubeconfig is required")
	case cfg.context == "":
		return cfg, errors.New("--context is required")
	case cfg.namespace == "":
		return cfg, errors.New("--namespace is required")
	case len(cfg.resources) == 0:
		return cfg, errors.New("at least one --resource is required")
	case cfg.refresh < 100*time.Millisecond:
		return cfg, errors.New("--refresh must be at least 100ms")
	case cfg.tail < 0:
		return cfg, errors.New("--tail cannot be negative")
	}
	return cfg, nil
}

func (d *dashboard) kubectlArgs(args ...string) []string {
	base := []string{
		"--kubeconfig", d.cfg.kubeconfig,
		"--context", d.cfg.context,
		"--namespace", d.cfg.namespace,
	}
	return append(base, args...)
}

func (d *dashboard) kubectlJSON(ctx context.Context, out any, args ...string) error {
	if len(args) == 0 || args[0] != "get" {
		return fmt.Errorf("read-only guard rejected kubectl operation: %s", strings.Join(args, " "))
	}

	commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, d.cfg.kubectl, d.kubectlArgs(args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("kubectl %s: %s", strings.Join(args, " "), oneLine(message))
	}
	if err := json.Unmarshal(stdout.Bytes(), out); err != nil {
		return fmt.Errorf("decode kubectl %s output: %w", strings.Join(args, " "), err)
	}
	return nil
}

func (d *dashboard) poll(ctx context.Context) error {
	var current workloadCollection
	workloadArgs := append([]string{"get"}, d.cfg.resources...)
	workloadArgs = append(workloadArgs, "-o", "json")
	if err := d.kubectlJSON(ctx, &current, workloadArgs...); err != nil {
		return err
	}

	var allPods podList
	if err := d.kubectlJSON(ctx, &allPods, "get", "pods", "-o", "json"); err != nil {
		return err
	}

	views := make([]workloadView, 0, len(current.Items))
	podsByName := make(map[string]pod)
	for _, item := range current.Items {
		if len(item.Spec.Selector.MatchLabels) == 0 &&
			len(item.Spec.Selector.MatchExpressions) == 0 {
			return fmt.Errorf("%s/%s has no selector", strings.ToLower(item.Kind), item.Metadata.Name)
		}

		matchingPods := make([]pod, 0)
		for _, candidate := range allPods.Items {
			if matchesSelector(
				candidate.Metadata.Labels,
				item.Spec.Selector.MatchLabels,
				item.Spec.Selector.MatchExpressions,
			) {
				matchingPods = append(matchingPods, candidate)
				podsByName[candidate.Metadata.Name] = candidate
			}
		}
		views = append(views, workloadView{
			workload: item,
			summary:  summarize(item, matchingPods),
		})
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].workload.Metadata.Name < views[j].workload.Metadata.Name
	})
	pods := make([]pod, 0, len(podsByName))
	for _, item := range podsByName {
		pods = append(pods, item)
	}
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Metadata.CreationTimestamp.Before(pods[j].Metadata.CreationTimestamp)
	})

	d.workloads = views
	d.pods = pods
	d.summary = aggregateSummaries(views)
	d.lastRefresh = time.Now()
	d.recordLifecycle(pods)
	d.syncLogStreams(ctx, pods)
	return nil
}

func matchesSelector(
	labels map[string]string,
	matchLabels map[string]string,
	expressions []labelSelectorRequirement,
) bool {
	for key, value := range matchLabels {
		actual, exists := labels[key]
		if !exists || actual != value {
			return false
		}
	}
	for _, expression := range expressions {
		value, exists := labels[expression.Key]
		switch expression.Operator {
		case "In":
			if !exists || !contains(expression.Values, value) {
				return false
			}
		case "NotIn":
			if exists && contains(expression.Values, value) {
				return false
			}
		case "Exists":
			if !exists {
				return false
			}
		case "DoesNotExist":
			if exists {
				return false
			}
		case "Gt", "Lt":
			if !exists || len(expression.Values) == 0 {
				return false
			}
			actual, actualErr := strconv.Atoi(value)
			expected, expectedErr := strconv.Atoi(expression.Values[0])
			if actualErr != nil || expectedErr != nil {
				return false
			}
			if expression.Operator == "Gt" && actual <= expected {
				return false
			}
			if expression.Operator == "Lt" && actual >= expected {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func labelSelector(labels map[string]string, expressions []labelSelectorRequirement) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	for _, expression := range expressions {
		values := append([]string(nil), expression.Values...)
		sort.Strings(values)
		switch expression.Operator {
		case "In":
			parts = append(parts, expression.Key+" in ("+strings.Join(values, ",")+")")
		case "NotIn":
			parts = append(parts, expression.Key+" notin ("+strings.Join(values, ",")+")")
		case "Exists":
			parts = append(parts, expression.Key)
		case "DoesNotExist":
			parts = append(parts, "!"+expression.Key)
		case "Gt":
			if len(values) > 0 {
				parts = append(parts, expression.Key+">"+values[0])
			}
		case "Lt":
			if len(values) > 0 {
				parts = append(parts, expression.Key+"<"+values[0])
			}
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func summarize(w workload, pods []pod) rolloutSummary {
	var result rolloutSummary
	kind := strings.ToLower(w.Kind)

	switch kind {
	case "daemonset":
		result.Desired = w.Status.DesiredNumberScheduled
		result.Updated = w.Status.UpdatedNumberScheduled
		result.Ready = w.Status.NumberReady
		result.Available = w.Status.NumberAvailable
		result.Unavailable = w.Status.NumberUnavailable
	default:
		result.Desired = 1
		if w.Spec.Replicas != nil {
			result.Desired = *w.Spec.Replicas
		}
		result.Updated = w.Status.UpdatedReplicas
		result.Ready = w.Status.ReadyReplicas
		result.Available = w.Status.AvailableReplicas
		result.Unavailable = w.Status.UnavailableReplicas
	}

	for _, cond := range w.Status.Conditions {
		if cond.Type == "Progressing" && cond.Status == "False" {
			result.State = "FAILED"
			result.Message = firstNonEmpty(cond.Reason, cond.Message)
		}
		if cond.Type == "Available" && cond.Status == "False" && result.Message == "" {
			result.Message = firstNonEmpty(cond.Reason, cond.Message)
		}
	}

	for _, item := range pods {
		status := podDisplayStatus(item)
		if isPodFailure(status) && result.State == "" {
			result.State = "DEGRADED"
			result.Message = item.Metadata.Name + ": " + status
		}
	}

	if result.State == "" {
		generationObserved := w.Status.ObservedGeneration >= w.Metadata.Generation
		countsReady := result.Updated == result.Desired &&
			result.Ready == result.Desired &&
			result.Available == result.Desired &&
			result.Unavailable == 0
		if generationObserved && countsReady {
			result.State = "OK"
			result.Message = "All desired pods are updated and available"
		} else {
			result.State = "ROLLING"
			if !generationObserved {
				result.Message = "Waiting for the controller to observe the new generation"
			} else {
				result.Message = "Waiting for updated pods to become ready"
			}
		}
	}
	return result
}

func aggregateSummaries(views []workloadView) rolloutSummary {
	result := rolloutSummary{State: "OK"}
	priority := map[string]int{"OK": 1, "ROLLING": 2, "DEGRADED": 3, "FAILED": 4}
	problems := make([]string, 0)

	for _, view := range views {
		summary := view.summary
		result.Desired += summary.Desired
		result.Updated += summary.Updated
		result.Ready += summary.Ready
		result.Available += summary.Available
		result.Unavailable += summary.Unavailable
		if priority[summary.State] > priority[result.State] {
			result.State = summary.State
		}
		if summary.State != "OK" {
			problems = append(problems, view.workload.Metadata.Name+": "+summary.State)
		}
	}

	switch {
	case len(views) == 0:
		result.State = "DEGRADED"
		result.Message = "No workloads returned by Kubernetes"
	case len(problems) == 0:
		result.Message = fmt.Sprintf("All %d workloads are updated and available", len(views))
	default:
		result.Message = strings.Join(problems, " · ")
	}
	return result
}

func podDisplayStatus(item pod) string {
	if item.Metadata.DeletionTimestamp != nil {
		return "Terminating"
	}
	for _, container := range item.Status.ContainerStatuses {
		if container.State.Waiting != nil && container.State.Waiting.Reason != "" {
			return container.State.Waiting.Reason
		}
		if container.State.Terminated != nil && container.State.Terminated.Reason != "Completed" {
			reason := container.State.Terminated.Reason
			if reason == "" {
				reason = "ExitCode " + strconv.Itoa(container.State.Terminated.ExitCode)
			}
			return reason
		}
	}
	if item.Status.Reason != "" {
		return item.Status.Reason
	}
	if item.Status.Phase != "" {
		return item.Status.Phase
	}
	return "Unknown"
}

func isPodFailure(status string) bool {
	switch status {
	case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError",
		"RunContainerError", "Error", "Failed", "Evicted", "OOMKilled":
		return true
	default:
		return strings.HasPrefix(status, "ExitCode ")
	}
}

func podReady(item pod) (int, int) {
	ready := 0
	for _, container := range item.Status.ContainerStatuses {
		if container.Ready {
			ready++
		}
	}
	return ready, len(item.Status.ContainerStatuses)
}

func podRestarts(item pod) int {
	total := 0
	for _, container := range item.Status.ContainerStatuses {
		total += container.RestartCount
	}
	return total
}

func (d *dashboard) recordLifecycle(current []pod) {
	next := make(map[string]string, len(current))
	now := time.Now().Format("15:04:05")

	for _, item := range current {
		ready, total := podReady(item)
		state := fmt.Sprintf("%s · ready %d/%d · restarts %d",
			podDisplayStatus(item), ready, total, podRestarts(item))
		next[item.Metadata.Name] = state

		previous, existed := d.previousPods[item.Metadata.Name]
		switch {
		case !existed:
			d.lifecycle = appendBounded(d.lifecycle,
				fmt.Sprintf("%s  + %s appeared · %s", now, item.Metadata.Name, state), 100)
		case previous != state:
			d.lifecycle = appendBounded(d.lifecycle,
				fmt.Sprintf("%s  ~ %s · %s → %s", now, item.Metadata.Name, previous, state), 100)
		}
	}

	for name, previous := range d.previousPods {
		if _, exists := next[name]; !exists {
			d.lifecycle = appendBounded(d.lifecycle,
				fmt.Sprintf("%s  - %s disappeared · was %s", now, name, previous), 100)
		}
	}
	d.previousPods = next
}

func (d *dashboard) syncLogStreams(ctx context.Context, pods []pod) {
	present := make(map[string]bool, len(pods))
	for _, item := range pods {
		name := item.Metadata.Name
		present[name] = true
		if item.Metadata.DeletionTimestamp != nil {
			continue
		}
		if _, running := d.streams[name]; running {
			continue
		}
		streamCtx, cancel := context.WithCancel(ctx)
		d.streams[name] = logStream{cancel: cancel}
		go d.followLogs(streamCtx, name)
	}

	for name, stream := range d.streams {
		if !present[name] {
			stream.cancel()
			delete(d.streams, name)
		}
	}
}

func (d *dashboard) followLogs(ctx context.Context, podName string) {
	defer func() {
		select {
		case d.streamDone <- podName:
		default:
		}
	}()

	args := d.kubectlArgs(
		"logs", "-f", "pod/"+podName,
		"--all-containers=true",
		"--prefix=true",
		fmt.Sprintf("--tail=%d", d.cfg.tail),
	)
	cmd := exec.CommandContext(ctx, d.cfg.kubectl, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		d.sendLog(ctx, podName, "log stream error: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		d.sendLog(ctx, podName, "log stream error: "+err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		d.sendLog(ctx, podName, "log stream error: "+err.Error())
		return
	}

	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		d.scanLogs(ctx, podName, stdout, false)
	}()
	go func() {
		defer readers.Done()
		d.scanLogs(ctx, podName, stderr, true)
	}()
	readers.Wait()
	err = cmd.Wait()
	if err != nil && ctx.Err() == nil {
		d.sendLog(ctx, podName, "log stream ended: "+err.Error())
	}
}

func (d *dashboard) scanLogs(ctx context.Context, podName string, reader io.Reader, stderr bool) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if stderr {
			line = "! " + line
		}
		d.sendLog(ctx, podName, line)
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		d.sendLog(ctx, podName, "log read error: "+err.Error())
	}
}

func (d *dashboard) sendLog(ctx context.Context, podName, text string) {
	select {
	case d.logLines <- logLine{pod: podName, text: oneLine(text), at: time.Now()}:
	case <-ctx.Done():
	}
}

func (d *dashboard) stopStreams() {
	for _, stream := range d.streams {
		stream.cancel()
	}
}

func (d *dashboard) render() {
	var out strings.Builder
	out.WriteString("\x1b[H\x1b[2J")

	target := d.cfg.resources.String()
	if len(d.cfg.resources) > 1 {
		target = fmt.Sprintf("Kubernetes suite · %d workloads", len(d.cfg.resources))
	}
	header := fmt.Sprintf(" KUBERNETES DEPLOY WATCH · READ ONLY  %s  %s  %s ",
		strings.ToUpper(d.cfg.environment), d.cfg.namespace, target)
	out.WriteString(colorMagenta + bold(fit(header, d.width)) + colorReset + "\n")

	stateColor := colorYellow
	switch d.summary.State {
	case "OK":
		stateColor = colorGreen
	case "FAILED", "DEGRADED":
		stateColor = colorRed
	}
	counts := fmt.Sprintf(
		"%-8s desired %d  updated %d  ready %d  available %d  unavailable %d",
		d.summary.State,
		d.summary.Desired, d.summary.Updated, d.summary.Ready,
		d.summary.Available, d.summary.Unavailable,
	)
	out.WriteString(stateColor + fit(counts, d.width) + colorReset + "\n")
	out.WriteString(colorDim + fit(d.summary.Message, d.width) + colorReset + "\n")

	refresh := "waiting for Kubernetes"
	if !d.lastRefresh.IsZero() {
		refresh = "updated " + d.lastRefresh.Format("15:04:05") +
			" · every " + d.cfg.refresh.String()
	}
	out.WriteString(colorDim + fit(
		fmt.Sprintf("%s · context %s · Ctrl-C to quit", refresh, d.cfg.context),
		d.width,
	) + colorReset + "\n")
	if d.lastError != "" {
		out.WriteString(colorRed + fit("ERROR: "+d.lastError, d.width) + colorReset + "\n")
	} else {
		out.WriteString(strings.Repeat("─", max(1, d.width)) + "\n")
	}

	workloadRows := min(max(1, len(d.workloads)), 10)
	fixedWithoutLists := 11 + workloadRows
	availableRows := max(8, d.height-fixedWithoutLists)
	podRows := min(max(3, len(d.pods)), min(10, availableRows/3))
	eventRows := min(5, max(2, availableRows/5))
	logRows := max(3, availableRows-podRows-eventRows)

	out.WriteString(colorCyan + bold("WORKLOADS") + colorReset + "\n")
	out.WriteString(fit(
		fmt.Sprintf("  %-36s %-10s %7s %7s %7s %9s",
			"NAME", "STATE", "DESIRED", "UPDATED", "READY", "AVAILABLE"),
		d.width,
	) + "\n")
	if len(d.workloads) == 0 {
		out.WriteString(colorDim + "  waiting for workloads…\n" + colorReset)
	} else {
		for _, view := range d.workloads[:min(len(d.workloads), workloadRows)] {
			summary := view.summary
			row := fit(fmt.Sprintf("  %-36s %-10s %7d %7d %7d %9d",
				view.workload.Metadata.Name,
				summary.State,
				summary.Desired,
				summary.Updated,
				summary.Ready,
				summary.Available,
			), d.width)
			switch summary.State {
			case "OK":
				out.WriteString(colorGreen + row + colorReset + "\n")
			case "FAILED", "DEGRADED":
				out.WriteString(colorRed + row + colorReset + "\n")
			default:
				out.WriteString(colorYellow + row + colorReset + "\n")
			}
		}
	}

	out.WriteString(colorCyan + bold("PODS") + colorReset + "\n")
	out.WriteString(fit(
		fmt.Sprintf("  %-42s %-18s %-7s %-8s %-8s", "NAME", "STATUS", "READY", "RESTARTS", "AGE"),
		d.width,
	) + "\n")
	if len(d.pods) == 0 {
		out.WriteString(colorDim + "  no matching pods\n" + colorReset)
	} else {
		start := max(0, len(d.pods)-podRows)
		for _, item := range d.pods[start:] {
			status := podDisplayStatus(item)
			ready, total := podReady(item)
			row := fmt.Sprintf("  %-42s %-18s %d/%-5d %-8d %-8s",
				item.Metadata.Name,
				status,
				ready,
				total,
				podRestarts(item),
				shortAge(item.Metadata.CreationTimestamp),
			)
			row = fit(row, d.width)
			switch {
			case isPodFailure(status):
				out.WriteString(colorRed + row + colorReset + "\n")
			case status == "Terminating" || status == "Pending":
				out.WriteString(colorYellow + row + colorReset + "\n")
			case status == "Running" && ready == total && total > 0:
				out.WriteString(colorGreen + row + colorReset + "\n")
			default:
				out.WriteString(row + "\n")
			}
		}
	}

	out.WriteString(colorCyan + bold("LIFECYCLE") + colorReset + "\n")
	writeTail(&out, d.lifecycle, eventRows, d.width, "  no pod changes observed yet")

	out.WriteString(colorCyan + bold("LIVE LOGS") + colorReset + "\n")
	if len(d.logs) == 0 {
		out.WriteString(colorDim + "  waiting for pod logs…\n" + colorReset)
	} else {
		renderLogTail(&out, d.logs, logRows, d.width)
	}

	fmt.Print(out.String())
}

func renderLogTail(out *strings.Builder, lines []logLine, rows, width int) {
	bodyWidth := max(1, width-39)
	start := len(lines)
	usedRows := 0
	for start > 0 {
		parts := wrapText(lines[start-1].text, bodyWidth)
		if usedRows > 0 && usedRows+len(parts) > rows {
			break
		}
		usedRows += len(parts)
		start--
		if usedRows >= rows {
			break
		}
	}

	writtenRows := 0
	for _, line := range lines[start:] {
		timestamp := line.at.Format("15:04:05")
		podName := fmt.Sprintf("%-28s", compactPodName(line.pod, 28))
		for partIndex, body := range wrapText(line.text, bodyWidth) {
			if writtenRows >= rows {
				return
			}
			if partIndex == 0 {
				out.WriteString(
					colorDim + timestamp + colorReset + " " +
						colorMagenta + podName + colorReset + " " +
						logColor(line.text) + body + colorReset + "\n",
				)
			} else {
				out.WriteString(strings.Repeat(" ", 39) + logColor(line.text) + body + colorReset + "\n")
			}
			writtenRows++
		}
	}
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > 0 {
		end := min(width, len(runes))
		lines = append(lines, string(runes[:end]))
		runes = runes[end:]
	}
	return lines
}

func writeTail(out *strings.Builder, lines []string, count, width int, empty string) {
	if len(lines) == 0 {
		out.WriteString(colorDim + empty + colorReset + "\n")
		return
	}
	start := max(0, len(lines)-count)
	for _, line := range lines[start:] {
		color := ""
		switch {
		case strings.Contains(line, " - "):
			color = colorRed
		case strings.Contains(line, " + "):
			color = colorGreen
		case strings.Contains(line, " ~ "):
			color = colorYellow
		}
		out.WriteString(color + fit(line, width) + colorReset + "\n")
	}
}

func terminalSize() (int, int) {
	width, height := 120, 36
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	output, err := cmd.Output()
	if err == nil {
		fields := strings.Fields(string(output))
		if len(fields) == 2 {
			if value, parseErr := strconv.Atoi(fields[0]); parseErr == nil && value > 0 {
				height = value
			}
			if value, parseErr := strconv.Atoi(fields[1]); parseErr == nil && value > 0 {
				width = value
			}
		}
	}
	return width, height
}

func shortAge(created time.Time) string {
	if created.IsZero() {
		return "?"
	}
	age := time.Since(created)
	if age < 0 {
		age = 0
	}
	switch {
	case age < time.Minute:
		return fmt.Sprintf("%ds", int(age.Seconds()))
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	}
}

func compactPodName(name string, width int) string {
	if utf8.RuneCountInString(name) <= width {
		return name
	}
	if width < 4 {
		return fit(name, width)
	}
	runes := []rune(name)
	return string(runes[:width-1]) + "…"
}

func fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(value)
	return string(runes[:width-1]) + "…"
}

func bold(value string) string {
	return "\x1b[1m" + value + "\x1b[22m"
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func logColor(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "fatal"),
		strings.Contains(lower, "panic"),
		strings.Contains(lower, "error"),
		strings.Contains(lower, "exception"),
		strings.Contains(lower, "traceback"),
		strings.HasPrefix(lower, "! "):
		return colorRed
	case strings.Contains(lower, "warn"):
		return colorYellow
	case strings.Contains(lower, "debug"),
		strings.Contains(lower, "trace"):
		return colorDim
	case strings.Contains(lower, "info"),
		strings.Contains(lower, "notice"):
		return colorGreen
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendBounded[T any](items []T, item T, limit int) []T {
	items = append(items, item)
	if len(items) <= limit {
		return items
	}
	return append([]T(nil), items[len(items)-limit:]...)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
