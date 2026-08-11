package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	colorReset     = "\x1b[0m"
	colorDim       = "\x1b[2m"
	colorRed       = "\x1b[31m"
	colorDimRed    = "\x1b[2;31m"
	colorGreen     = "\x1b[32m"
	colorYellow    = "\x1b[33m"
	colorBlue      = "\x1b[34m"
	colorMagenta   = "\x1b[35m"
	colorCyan      = "\x1b[36m"
	colorHighlight = "\x1b[30;103m"
)

var (
	structuredLevelPattern = regexp.MustCompile(
		`(?i)(?:\[\s*(fatal|critical|panic|error|err|warning|warn|info|debug|trace)\s*\]|` +
			`"(?:level|severity)"\s*:\s*"(fatal|critical|panic|error|err|warning|warn|info|debug|trace)"|` +
			`(?:^|\s)(?:level|severity)=(fatal|critical|panic|error|err|warning|warn|info|debug|trace)(?:\s|$))`,
	)
	plainFailurePattern = regexp.MustCompile(
		`(?i)(?:^|[\s:])(fatal|critical|panic|error|err|exception|traceback)(?:[\s:]|$)`,
	)
	exceptionLinePattern = regexp.MustCompile(
		`(?i)^\s*[\w.]*?(error|exception):(?:\s|$)`,
	)
	stackTraceLinePattern = regexp.MustCompile(
		`(?i)^\s*(?:traceback \(most recent call last\):|` +
			`file ".*", line \d+|` +
			`(?:return|raise|async for|do\s*=|result\s*=|self\.)\s|` +
			`[\^~]+\s*$)`,
	)
	durationPattern = regexp.MustCompile(
		`(?i)\b(?:duration|duration_ms|execution_time_ms|elapsed|elapsed_ms|latency|latency_ms)=[0-9]+(?:\.[0-9]+)?`,
	)
	statusCodePattern = regexp.MustCompile(
		`(?i)\b(?:status|status_code|http_status)=([1-5][0-9]{2})\b`,
	)
	httpStatusPattern = regexp.MustCompile(`(?i)\bHTTP\s+([1-5][0-9]{2})\b`)
	methodPattern     = regexp.MustCompile(
		`(?i)\bmethod=(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\b`,
	)
)

type stringList []string

func (items *stringList) String() string {
	return strings.Join(*items, ",")
}

func (items *stringList) Set(value string) error {
	if value == "" || strings.Contains(value, "/") {
		return errors.New("pod must be a plain Kubernetes pod name")
	}
	if !contains(*items, value) {
		*items = append(*items, value)
	}
	return nil
}

type config struct {
	kubectl     string
	kubeconfig  string
	context     string
	namespace   string
	environment string
	pods        stringList
	query       string
	filter      bool
	tail        int
	buffer      int
}

type logLine struct {
	pod       string
	container string
	text      string
	at        time.Time
}

type viewer struct {
	cfg           config
	lines         []logLine
	pauseBuffer   []logLine
	aliases       map[string]string
	query         string
	draft         string
	filter        bool
	editing       bool
	paused        bool
	details       bool
	cursorFromEnd int
	escapeState   int
	width         int
	height        int
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	restore, err := rawTerminal()
	if err != nil {
		fmt.Fprintln(os.Stderr, "interactive terminal required:", err)
		os.Exit(2)
	}
	defer restore()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logs := make(chan logLine, 512)
	var streams sync.WaitGroup
	for _, pod := range cfg.pods {
		streams.Add(1)
		go func() {
			defer streams.Done()
			followPodLogs(ctx, cfg, pod, logs)
		}()
	}

	input := make(chan rune, 32)
	go readInput(ctx, input)

	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)

	current := viewer{
		cfg:     cfg,
		aliases: podAliases(cfg.pods),
		query:   cfg.query,
		filter:  cfg.filter,
	}
	current.width, current.height = terminalSize()

	fmt.Print("\x1b[?1049h\x1b[?25l")
	defer fmt.Print("\x1b[?25h\x1b[?1049l")
	current.render()

	renderTicker := time.NewTicker(100 * time.Millisecond)
	defer renderTicker.Stop()
	dirty := false

	for {
		select {
		case <-ctx.Done():
			waitForStreams(&streams, 2*time.Second)
			return
		case line := <-logs:
			current.addLine(line)
			dirty = true
		case key := <-input:
			if current.handleKey(key) {
				cancel()
				continue
			}
			dirty = true
		case <-resize:
			current.width, current.height = terminalSize()
			dirty = true
		case <-renderTicker.C:
			if dirty {
				current.render()
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
	flag.Var(&cfg.pods, "pod", "pod name (repeatable)")
	flag.StringVar(&cfg.query, "query", "", "initial highlight query")
	flag.BoolVar(&cfg.filter, "filter", false, "initially show only matching lines")
	flag.IntVar(&cfg.tail, "tail", 100, "initial log lines per pod")
	flag.IntVar(&cfg.buffer, "buffer", 5000, "maximum log lines kept in memory")
	flag.Parse()

	switch {
	case cfg.kubeconfig == "":
		return cfg, errors.New("--kubeconfig is required")
	case cfg.context == "":
		return cfg, errors.New("--context is required")
	case cfg.namespace == "":
		return cfg, errors.New("--namespace is required")
	case len(cfg.pods) == 0:
		return cfg, errors.New("at least one --pod is required")
	case cfg.tail < 0:
		return cfg, errors.New("--tail cannot be negative")
	case cfg.buffer < 100:
		return cfg, errors.New("--buffer must be at least 100")
	}
	return cfg, nil
}

func followPodLogs(ctx context.Context, cfg config, pod string, output chan<- logLine) {
	args := logArgs(cfg, pod)
	cmd := exec.CommandContext(ctx, cfg.kubectl, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sendLog(ctx, output, pod, "log stream error: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		sendLog(ctx, output, pod, "log stream error: "+err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		sendLog(ctx, output, pod, "log stream error: "+err.Error())
		return
	}

	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		scanLogs(ctx, output, pod, stdout, false)
	}()
	go func() {
		defer readers.Done()
		scanLogs(ctx, output, pod, stderr, true)
	}()
	readers.Wait()
	err = cmd.Wait()
	if err != nil && ctx.Err() == nil {
		sendLog(ctx, output, pod, "log stream ended: "+err.Error())
	}
}

func logArgs(cfg config, pod string) []string {
	return []string{
		"--kubeconfig", cfg.kubeconfig,
		"--context", cfg.context,
		"--namespace", cfg.namespace,
		"logs", "-f", "pod/" + pod,
		"--all-containers=true",
		"--prefix=true",
		fmt.Sprintf("--tail=%d", cfg.tail),
	}
}

func scanLogs(
	ctx context.Context,
	output chan<- logLine,
	pod string,
	reader io.Reader,
	stderr bool,
) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if stderr {
			line = "! " + line
		}
		sendLog(ctx, output, pod, line)
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		sendLog(ctx, output, pod, "log read error: "+err.Error())
	}
}

func sendLog(ctx context.Context, output chan<- logLine, pod, text string) {
	container, body := normalizeKubectlLine(pod, sanitize(text))
	select {
	case output <- logLine{
		pod:       pod,
		container: container,
		text:      body,
		at:        time.Now(),
	}:
	case <-ctx.Done():
	}
}

func readInput(ctx context.Context, output chan<- rune) {
	reader := bufio.NewReader(os.Stdin)
	for {
		key, _, err := reader.ReadRune()
		if err != nil {
			return
		}
		select {
		case output <- key:
		case <-ctx.Done():
			return
		}
	}
}

func (v *viewer) handleKey(key rune) bool {
	if v.editing {
		switch key {
		case '\r', '\n':
			v.query = strings.TrimSpace(v.draft)
			v.editing = false
			v.clampCursor()
		case 27:
			v.draft = v.query
			v.editing = false
		case 8, 127:
			runes := []rune(v.draft)
			if len(runes) > 0 {
				v.draft = string(runes[:len(runes)-1])
			}
		default:
			if unicode.IsPrint(key) {
				v.draft += string(key)
			}
		}
		return false
	}

	if v.escapeState == 1 {
		v.escapeState = 0
		if key == '[' {
			v.escapeState = 2
			return false
		}
	}
	if v.escapeState == 2 {
		v.escapeState = 0
		switch key {
		case 'A':
			v.moveOlder()
		case 'B':
			v.moveNewer()
		}
		return false
	}
	if key == 27 {
		v.escapeState = 1
		return false
	}

	switch key {
	case '/':
		v.draft = v.query
		v.editing = true
	case ' ', 'p', 'P':
		v.togglePause()
	case 'f', 'F':
		v.filter = !v.filter
		v.clampCursor()
	case 'c', 'C':
		v.query = ""
		v.draft = ""
		v.filter = false
		v.clampCursor()
	case 'k', 'K':
		v.moveOlder()
	case 'j', 'J':
		v.moveNewer()
	case '\r', '\n':
		if v.paused {
			v.details = !v.details
		}
	case 'q', 'Q':
		return true
	}
	return false
}

func (v *viewer) addLine(line logLine) {
	v.lines = appendBounded(v.lines, line, v.cfg.buffer)
	if !v.paused {
		v.clampCursor()
	}
}

func (v *viewer) togglePause() {
	v.details = false
	if v.paused {
		v.paused = false
		v.pauseBuffer = nil
		v.cursorFromEnd = 0
		return
	}

	v.paused = true
	v.pauseBuffer = append([]logLine(nil), v.lines...)
	v.cursorFromEnd = 0
	v.clampCursor()
}

func (v *viewer) moveOlder() {
	if !v.paused {
		v.togglePause()
	}
	v.cursorFromEnd++
	v.clampCursor()
}

func (v *viewer) moveNewer() {
	if !v.paused {
		return
	}
	v.cursorFromEnd = max(0, v.cursorFromEnd-1)
	v.clampCursor()
}

func (v *viewer) clampCursor() {
	visible := v.visibleLines(v.activeQuery())
	v.cursorFromEnd = min(max(0, v.cursorFromEnd), max(0, len(visible)-1))
}

func (v *viewer) activeQuery() string {
	if v.editing {
		return v.draft
	}
	return v.query
}

func (v *viewer) lineVisible(line logLine, terms []string) bool {
	return !v.filter || len(terms) == 0 || matchesAll(line.text, terms)
}

func (v *viewer) visibleLines(query string) []logLine {
	terms := queryTerms(query)
	source := v.lines
	if v.paused {
		source = v.pauseBuffer
	}
	visible := make([]logLine, 0, len(source))
	for _, line := range source {
		if v.lineVisible(line, terms) {
			visible = append(visible, line)
		}
	}
	return visible
}

func (v *viewer) render() {
	var out strings.Builder
	out.WriteString("\x1b[H\x1b[2J")

	state := "LIVE"
	if v.paused {
		state = fmt.Sprintf("PAUSED · +%d buffered", max(0, len(v.lines)-len(v.pauseBuffer)))
	}
	header := fmt.Sprintf(
		" PODBOR MULTI LOGS · READ ONLY  %s/%s  %d pods  %s ",
		strings.ToUpper(v.cfg.environment),
		v.cfg.namespace,
		len(v.cfg.pods),
		state,
	)
	out.WriteString(colorMagenta + bold(fit(header, v.width)) + colorReset + "\n")

	query := v.activeQuery()
	mode := "highlight"
	if v.filter {
		mode = "ONLY MATCHES"
	}
	queryStatus := "(none)"
	if query != "" {
		queryStatus = query
	}
	out.WriteString(fit(
		fmt.Sprintf(" %s · words: %s", mode, queryStatus),
		v.width,
	) + "\n")

	out.WriteString(strings.Repeat("─", max(1, v.width)) + "\n")

	terms := queryTerms(query)
	visible := v.visibleLines(query)
	rows := max(1, v.height-5)
	if len(visible) == 0 {
		message := " waiting for logs…"
		if len(v.lines) > 0 && v.filter && len(terms) > 0 {
			message = " no buffered lines match all words"
		}
		out.WriteString(colorDim + fit(message, v.width) + colorReset + "\n")
	} else if v.paused && v.details {
		v.renderDetails(&out, visible, rows, terms)
	} else {
		v.renderList(&out, visible, rows, terms)
	}

	out.WriteString(strings.Repeat("─", max(1, v.width)) + "\n")
	help := "[/] words  [Space/p] pause  [f] only matches  [c] clear  [q] quit"
	switch {
	case v.editing:
		help = "FILTER WORDS › " + v.draft + "█   [Enter] apply  [Esc] cancel"
	case v.paused && v.details:
		help = "[j/k or ↑/↓] line  [Enter] back to list  [Space/p] resume  [q] quit"
	case v.paused:
		help = "[j/k or ↑/↓] line  [Enter] full details  [Space/p] resume  [/] words"
	}
	out.WriteString(colorCyan + fit(" "+help, v.width) + colorReset + "\n")
	fmt.Print(out.String())
}

func (v *viewer) renderList(
	out *strings.Builder,
	visible []logLine,
	rows int,
	terms []string,
) {
	selected := len(visible) - 1
	if v.paused {
		selected = max(0, len(visible)-1-v.cursorFromEnd)
	}
	start := max(0, len(visible)-rows)
	if v.paused {
		start = min(max(0, selected-rows/2), max(0, len(visible)-rows))
	}
	end := min(len(visible), start+rows)

	for index := start; index < end; index++ {
		line := visible[index]
		marker := " "
		if v.paused && index == selected {
			marker = "›"
		}
		alias := compact(v.aliases[line.pod], 12)
		prefix := marker + " " + fmt.Sprintf("%-12s", alias) + " "
		body := fit(line.text, max(1, v.width-visibleWidth(prefix)))
		out.WriteString(
			colorCyan + marker + colorReset + " " +
				colorMagenta + fmt.Sprintf("%-12s", alias) + colorReset + " " +
				highlight(body, terms, severityColor(body)) + colorReset + "\n",
		)
	}
}

func (v *viewer) renderDetails(
	out *strings.Builder,
	visible []logLine,
	rows int,
	terms []string,
) {
	selected := max(0, len(visible)-1-v.cursorFromEnd)
	line := visible[selected]
	metadata := fmt.Sprintf(
		" DETAIL  pod=%s  container=%s  received=%s ",
		line.pod,
		firstNonEmpty(line.container, "(unknown)"),
		line.at.Format("15:04:05.000"),
	)
	out.WriteString(colorMagenta + fit(metadata, v.width) + colorReset + "\n")

	wrapped := wrapText(line.text, max(1, v.width-2))
	available := max(1, rows-1)
	if len(wrapped) > available {
		wrapped = wrapped[:available]
	}
	for _, part := range wrapped {
		out.WriteString("  " + highlight(part, terms, severityColor(line.text)) + colorReset + "\n")
	}
}

func queryTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		if !contains(terms, field) {
			terms = append(terms, field)
		}
	}
	return terms
}

func matchesAll(text string, terms []string) bool {
	lower := strings.ToLower(text)
	for _, term := range terms {
		if !strings.Contains(lower, term) {
			return false
		}
	}
	return true
}

type styleSpan struct {
	start    int
	end      int
	color    string
	priority int
}

func highlight(text string, terms []string, baseColor string) string {
	spans := importantFieldSpans(text)
	if len(terms) > 0 {
		patterns := make([]string, 0, len(terms))
		for _, term := range terms {
			patterns = append(patterns, regexp.QuoteMeta(term))
		}
		matcher := regexp.MustCompile("(?i:" + strings.Join(patterns, "|") + ")")
		for _, match := range matcher.FindAllStringIndex(text, -1) {
			spans = append(spans, styleSpan{
				start: match[0], end: match[1], color: colorHighlight, priority: 100,
			})
		}
	}
	if len(spans) == 0 {
		return baseColor + text
	}

	boundaries := []int{0, len(text)}
	for _, span := range spans {
		boundaries = append(boundaries, span.start, span.end)
	}
	sort.Ints(boundaries)
	boundaries = uniqueInts(boundaries)

	var out strings.Builder
	activeColor := "\x00"
	for index := 0; index < len(boundaries)-1; index++ {
		start, end := boundaries[index], boundaries[index+1]
		if start == end {
			continue
		}
		color := baseColor
		priority := 0
		for _, span := range spans {
			if span.start <= start && span.end >= end && span.priority > priority {
				color = span.color
				priority = span.priority
			}
		}
		if color != activeColor {
			if activeColor != "\x00" {
				out.WriteString(colorReset)
			}
			out.WriteString(color)
			activeColor = color
		}
		out.WriteString(text[start:end])
	}
	return out.String()
}

func importantFieldSpans(text string) []styleSpan {
	spans := make([]styleSpan, 0)
	for _, match := range durationPattern.FindAllStringIndex(text, -1) {
		spans = append(spans, styleSpan{
			start: match[0], end: match[1], color: colorCyan, priority: 10,
		})
	}
	for _, match := range methodPattern.FindAllStringIndex(text, -1) {
		spans = append(spans, styleSpan{
			start: match[0], end: match[1], color: colorBlue, priority: 10,
		})
	}
	spans = append(spans, statusSpans(text, statusCodePattern)...)
	spans = append(spans, statusSpans(text, httpStatusPattern)...)
	return spans
}

func statusSpans(text string, pattern *regexp.Regexp) []styleSpan {
	spans := make([]styleSpan, 0)
	for _, match := range pattern.FindAllStringSubmatchIndex(text, -1) {
		if len(match) < 4 {
			continue
		}
		code, err := strconv.Atoi(text[match[2]:match[3]])
		if err != nil {
			continue
		}
		color := colorGreen
		switch {
		case code >= 500:
			color = colorRed
		case code >= 400:
			color = colorYellow
		case code >= 300:
			color = colorCyan
		}
		spans = append(spans, styleSpan{
			start: match[0], end: match[1], color: color, priority: 20,
		})
	}
	return spans
}

func uniqueInts(values []int) []int {
	if len(values) == 0 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func severityColor(text string) string {
	switch logLevel(text) {
	case "fatal", "critical", "panic", "error", "err", "exception", "traceback":
		return colorRed
	case "warn", "warning":
		return colorYellow
	case "debug", "trace":
		return colorDim
	case "stacktrace":
		return colorDimRed
	default:
		return ""
	}
}

func logLevel(text string) string {
	if match := structuredLevelPattern.FindStringSubmatch(text); len(match) > 1 {
		for _, level := range match[1:] {
			if level != "" {
				return strings.ToLower(level)
			}
		}
	}
	if match := plainFailurePattern.FindStringSubmatch(text); len(match) > 1 {
		for _, level := range match[1:] {
			if level != "" {
				return strings.ToLower(level)
			}
		}
	}
	if exceptionLinePattern.MatchString(text) {
		return "error"
	}
	if stackTraceLinePattern.MatchString(text) {
		return "stacktrace"
	}
	return ""
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sanitize(text string) string {
	return strings.Map(func(value rune) rune {
		switch {
		case value == '\t':
			return ' '
		case unicode.IsControl(value):
			return -1
		default:
			return value
		}
	}, text)
}

func normalizeKubectlLine(pod, text string) (string, string) {
	prefix := "[pod/" + pod + "/"
	if !strings.HasPrefix(text, prefix) {
		return "", text
	}
	closing := strings.Index(text[len(prefix):], "]")
	if closing < 0 {
		return "", text
	}
	containerStart := len(prefix)
	containerEnd := containerStart + closing
	container := text[containerStart:containerEnd]
	body := strings.TrimLeft(text[containerEnd+1:], " ")
	return container, body
}

func podAliases(pods []string) map[string]string {
	aliases := make(map[string]string, len(pods))
	if len(pods) == 1 {
		aliases[pods[0]] = pods[0]
		return aliases
	}

	prefix := pods[0]
	for _, pod := range pods[1:] {
		for !strings.HasPrefix(pod, prefix) && prefix != "" {
			prefix = prefix[:len(prefix)-1]
		}
	}
	if boundary := strings.LastIndex(prefix, "-"); boundary >= 0 {
		prefix = prefix[:boundary+1]
	}

	used := make(map[string]bool, len(pods))
	for _, pod := range pods {
		alias := strings.TrimPrefix(pod, prefix)
		if alias == "" || used[alias] {
			alias = pod
		}
		aliases[pod] = alias
		used[alias] = true
	}
	return aliases
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func rawTerminal() (func(), error) {
	stateCommand := exec.Command("stty", "-g")
	stateCommand.Stdin = os.Stdin
	state, err := stateCommand.Output()
	if err != nil {
		return nil, err
	}

	rawCommand := exec.Command("stty", "-echo", "-icanon", "min", "1", "time", "0")
	rawCommand.Stdin = os.Stdin
	if output, err := rawCommand.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	original := strings.TrimSpace(string(state))
	return func() {
		command := exec.Command("stty", original)
		command.Stdin = os.Stdin
		_ = command.Run()
	}, nil
}

func terminalSize() (int, int) {
	width, height := 120, 36
	command := exec.Command("stty", "size")
	command.Stdin = os.Stdin
	output, err := command.Output()
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

func waitForStreams(streams *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		streams.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func compact(value string, width int) string {
	if visibleWidth(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(value)
	return "…" + string(runes[max(0, len(runes)-width+1):])
}

func fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if visibleWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(value)
	return string(runes[:max(0, width-1)]) + "…"
}

func visibleWidth(value string) int {
	return utf8.RuneCountInString(value)
}

func bold(value string) string {
	return "\x1b[1m" + value + colorReset
}

func appendBounded[T any](items []T, item T, limit int) []T {
	items = append(items, item)
	if len(items) <= limit {
		return items
	}
	copy(items, items[len(items)-limit:])
	return items[:limit]
}
