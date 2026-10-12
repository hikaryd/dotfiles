package logwatch

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

	"dots/native/internal/kube/kubeui"
	"dots/native/internal/tui"
	"github.com/mattn/go-runewidth"
)

const (
	colorReset     = tui.Reset
	colorDim       = tui.Dim
	colorRed       = tui.Red
	colorDimRed    = tui.Faint + tui.Red
	colorGreen     = tui.Green
	colorYellow    = tui.Yellow
	colorBlue      = tui.Accent
	colorMagenta   = tui.Purple
	colorCyan      = tui.Accent
	colorHighlight = tui.Highlight
)

var (
	structuredLevelPattern = regexp.MustCompile(
		`(?i)(?:\[\s*(fatal|critical|panic|error|err|warning|warn|info|debug|trace)\s*\]|` +
			`"(?:level|severity)"\s*:\s*"(fatal|critical|panic|error|err|warning|warn|info|debug|trace)"|` +
			`(?:^|\s)(?:level|severity)=(fatal|critical|panic|error|err|warning|warn|info|debug|trace)(?:\s|$))`,
	)
	plainFailurePattern = regexp.MustCompile(
		`(?i)(?:^|[\s:])(fatal|critical|panic|error|err|exception|traceback|warning|warn|info|notice|debug|trace)(?:[\s:]|$)`,
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
	compact     bool
	tail        int
	buffer      int
	maxBytes    int
}

type logLine struct {
	pod       string
	container string
	text      string
	at        time.Time
}

type viewer struct {
	cfg             config
	lines           history
	total, pausedAt uint64
	pauseBuffer     []logLine
	query           string
	draft           string
	filter          bool
	compact         bool
	editing         bool
	paused          bool
	details         bool
	detailOffset    int
	cursorFromEnd   int
	width           int
	height          int
	painter         *tui.Painter
}

// Run starts the dashboard with an isolated flag set.
func Run(args []string) int {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if !tui.IsTerminal(os.Stdout) || !tui.IsTerminal(os.Stdin) {
		return streamPlain(ctx, cfg)
	}
	restore, err := rawTerminal()
	if err != nil {
		fmt.Fprintln(os.Stderr, "interactive terminal required:", err)
		return 2
	}
	defer restore()
	endScreen := tui.Screen(os.Stdout)
	defer endScreen()
	logs := make(chan logLine, 512)
	var streams sync.WaitGroup
	for _, pod := range cfg.pods {
		streams.Add(1)
		go func(pod string) { defer streams.Done(); followPodLogs(ctx, cfg, pod, logs) }(pod)
	}
	input := tui.Keys(ctx, os.Stdin)
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)
	current := viewer{cfg: cfg, query: cfg.query, filter: cfg.filter, compact: cfg.compact, lines: newHistory(cfg.buffer, cfg.maxBytes)}
	current.width, current.height = terminalSize()
	current.render()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	dirty := false
	lastDraw := time.Now()
	for {
		select {
		case <-ctx.Done():
			waitForStreams(&streams, 2*time.Second)
			return 0
		case line := <-logs:
			current.addLine(line)
			dirty = true
		case key, ok := <-input:
			if !ok || current.handleInput(key) {
				cancel()
				continue
			}
			current.render()
			dirty = false
			lastDraw = time.Now()
		case <-resize:
			current.width, current.height = terminalSize()
			dirty = true
		case <-ticker.C:
			if dirty && (!current.paused || time.Since(lastDraw) >= time.Second) {
				current.render()
				dirty = false
				lastDraw = time.Now()
			}
		}
	}
}
func (v *viewer) handleInput(key tui.Key) bool {
	switch key.Name {
	case "ctrl-c", "ctrl-d":
		return true
	case "esc":
		if v.editing {
			v.handleKey(27)
			return false
		}
		return true
	case "up":
		if v.details {
			v.detailOffset = max(0, v.detailOffset-1)
		} else {
			v.moveOlder()
		}
		return false
	case "down":
		if v.details {
			v.detailOffset++
		} else {
			v.moveNewer()
		}
		return false
	case "page-up":
		if v.details {
			v.detailOffset = max(0, v.detailOffset-max(1, v.height-9))
		} else {
			v.moveOlder()
		}
		return false
	case "page-down":
		if v.details {
			v.detailOffset += max(1, v.height-9)
		} else {
			v.moveNewer()
		}
		return false
	case "left":
		v.moveOlder()
		v.detailOffset = 0
		return false
	case "right":
		v.moveNewer()
		v.detailOffset = 0
		return false
	case "enter":
		return v.handleKey('\n')
	case "backspace":
		return v.handleKey(127)
	case "paste":
		if !v.editing {
			v.draft = v.query
			v.editing = true
		}
		v.draft += tui.Clean(key.Text)
		return false
	case "text":
		for _, r := range tui.Plain(key.Text) {
			if v.handleKey(r) {
				return true
			}
		}
	}
	return false
}

func parseConfig(args []string) (config, error) {
	flags := flag.NewFlagSet("kube-log-watch", flag.ContinueOnError)
	var cfg config
	flags.StringVar(&cfg.kubectl, "kubectl", "kubectl", "path to kubectl")
	flags.StringVar(&cfg.kubeconfig, "kubeconfig", "", "kubeconfig path")
	flags.StringVar(&cfg.context, "context", "", "Kubernetes context")
	flags.StringVar(&cfg.namespace, "namespace", "", "Kubernetes namespace")
	flags.StringVar(&cfg.environment, "environment", "", "short environment label")
	flags.Var(&cfg.pods, "pod", "pod name (repeatable)")
	flags.StringVar(&cfg.query, "query", "", "initial highlight query")
	flags.BoolVar(&cfg.filter, "filter", false, "initially show only matching lines")
	flags.BoolVar(&cfg.compact, "compact", false, "opt-in JSON message summaries instead of full records")
	flags.IntVar(&cfg.tail, "tail", 100, "initial log lines per pod")
	flags.IntVar(&cfg.buffer, "buffer", 5000, "maximum retained records")
	flags.IntVar(&cfg.maxBytes, "max-bytes", 32*1024*1024, "maximum retained log text bytes")
	if err := flags.Parse(args); err != nil {
		return cfg, err
	}

	switch {
	case cfg.context == "":
		return cfg, errors.New("--context is required")
	case cfg.namespace == "":
		return cfg, errors.New("--namespace is required")
	case len(cfg.pods) == 0:
		return cfg, errors.New("at least one --pod is required")
	case cfg.tail < 0:
		return cfg, errors.New("--tail cannot be negative")
	case cfg.buffer < 100 || cfg.buffer > 100000:
		return cfg, errors.New("--buffer must be between 100 and 100000")
	case cfg.maxBytes < 1024*1024 || cfg.maxBytes > 1024*1024*1024:
		return cfg, errors.New("--max-bytes must be between 1 MiB and 1 GiB")
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
	args := []string{
		"--context", cfg.context,
		"--namespace", cfg.namespace,
		"logs", "-f", "pod/" + pod,
		"--all-containers=true",
		"--prefix=true",
		fmt.Sprintf("--tail=%d", cfg.tail),
	}
	if cfg.kubeconfig != "" {
		args = append([]string{"--kubeconfig", cfg.kubeconfig}, args...)
	}
	return args
}

func scanLogs(
	ctx context.Context,
	output chan<- logLine,
	pod string,
	reader io.Reader,
	stderr bool,
) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
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

	switch key {
	case '/':
		v.draft = v.query
		v.editing = true
	case ' ', 'p', 'P':
		v.togglePause()
	case 'f', 'F':
		v.filter = !v.filter
		v.clampCursor()
	case 'm', 'M':
		v.compact = !v.compact
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
		if !v.paused {
			v.togglePause()
		}
		v.details = !v.details
		v.detailOffset = 0
	case 'q', 'Q':
		return true
	}
	return false
}

func (v *viewer) addLine(line logLine) {
	v.lines.push(line)
	v.total++
	if !v.paused {
		// Live mode always follows the tail. A full history/filter scan here
		// was quadratic in the buffer size at high incoming log rates.
		v.cursorFromEnd = 0
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
	v.pauseBuffer = v.lines.snapshot()
	v.pausedAt = v.total
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
	var source []logLine
	if v.paused {
		source = v.pauseBuffer
	} else {
		source = v.lines.snapshot()
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
	frame := v.frame(tui.ColorEnabled(os.Stdout))
	if v.painter == nil {
		v.painter = tui.NewPainter(os.Stdout)
	}
	v.painter.Draw(frame)
}

func (v *viewer) frame(color bool) tui.Frame {
	var out strings.Builder
	state := "LIVE"
	if v.paused {
		state = fmt.Sprintf("PAUSED · +%d received since pause", v.total-v.pausedAt)
	}
	query := v.activeQuery()
	mode := "highlight"
	if v.filter {
		mode = "ONLY MATCHES"
	}
	queryStatus := "(none)"
	if query != "" {
		queryStatus = query
	}
	view := "FULL RECORDS"
	if v.compact {
		view = "COMPACT SUMMARY"
	}
	out.WriteString(fit(fmt.Sprintf("%s · %s · %s · search: %s · %d held / %d retired", state, view, mode, queryStatus, v.lines.length, v.lines.dropped), v.width) + "\n")
	out.WriteString(strings.Repeat("─", max(1, v.width)) + "\n")
	terms := queryTerms(query)
	visible := v.visibleLines(query)
	rows := max(1, v.height-7)
	if len(v.cfg.pods) > 1 && v.width >= 60 && !v.details {
		out.WriteString(colorDim + fit(v.columns(visible).header(), v.width) + colorReset + "\n")
		out.WriteString(colorDim + strings.Repeat("─", max(1, v.width)) + colorReset + "\n")
		rows = max(1, rows-2)
	}
	if len(visible) == 0 {
		message := "Waiting for logs…"
		if v.lines.length > 0 && v.filter && len(terms) > 0 {
			message = "No buffered lines match the search"
		}
		out.WriteString(colorDim + fit(message, v.width) + colorReset + "\n")
	} else if v.paused && v.details {
		v.renderDetails(&out, visible, rows, terms)
	} else {
		v.renderList(&out, visible, rows, terms)
	}
	help := "/ search · Space pause · f filter · m compact/full · Enter details · Esc / q back"
	if v.editing {
		help = "Search › " + v.draft + "█ · Enter apply · Esc cancel"
	} else if v.paused {
		help = "↑↓ / j/k line · Enter details · Space resume · m compact/full · / search · Esc back"
	}
	if v.details && v.paused {
		help = "↑↓ / PgUp PgDn scroll · ←→ record · Enter back · Space resume · Esc / q exit"
	}
	title := fmt.Sprintf("Live logs · %d pods", len(v.cfg.pods))
	if len(v.cfg.pods) == 1 {
		title = "Pod logs · " + v.cfg.pods[0]
	}
	frame := kubeui.Frame(title, v.cfg.environment, v.cfg.context, v.cfg.namespace, "READ ONLY")
	frame.Width, frame.Height, frame.Color = v.width, v.height, color
	frame.Body, frame.Footer = out.String(), help
	return frame
}

func (v *viewer) renderList(out *strings.Builder, visible []logLine, rows int, terms []string) {
	selected := len(visible) - 1
	if v.paused {
		selected = max(0, len(visible)-1-v.cursorFromEnd)
	}
	end := len(visible)
	if v.paused {
		end = selected + 1
	}
	cards := len(v.cfg.pods) == 1 || v.width < 60
	columns := v.columns(visible)
	blocks := [][]string{}
	used := 0
	for index := end - 1; index >= 0; index-- {
		line := visible[index]
		marker := " "
		if v.paused && index == selected {
			marker = "›"
		}
		message := displayBody(line.text, v.compact)
		tone := severityColor(line.text)
		block := []string{}
		if cards {
			metadata := marker + " " + line.at.Format("15:04:05.000") + "  " + levelLabel(line.text) + "  " + firstNonEmpty(line.container, "log")
			block = append(block, colorDim+fit(metadata, v.width)+colorReset)
			if len(v.cfg.pods) > 1 {
				block = append(block, v.podTone(line.pod)+fit(line.pod, v.width)+colorReset)
			}
			parts := wrapCells(message, max(1, v.width-2))
			for _, part := range parts {
				block = append(block, "  "+highlight(part, terms, tone)+colorReset)
			}
			block = append(block, colorDim+strings.Repeat("─", max(1, v.width))+colorReset)
		} else {
			prefix := v.recordPrefix(line, marker, columns)
			parts := wrapCells(message, columns.message)
			for index, part := range parts {
				lead := prefix
				if index > 0 {
					lead = continuation(prefix)
				}
				block = append(block, lead+highlight(part, terms, tone)+colorReset)
			}
		}
		if used > 0 && used+len(block) > rows {
			break
		}
		if len(block) > rows {
			block = block[:rows]
			block[len(block)-1] = colorDim + fit(fmt.Sprintf("… %d display lines · Enter opens the full record", len(block)), v.width) + colorReset
		}
		blocks = append(blocks, block)
		used += len(block)
		if used >= rows {
			break
		}
	}
	for index := len(blocks) - 1; index >= 0; index-- {
		for _, line := range blocks[index] {
			out.WriteString(line + "\n")
		}
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

	wrapped := wrapCells(readableDetails(line.text), max(1, v.width-2))
	available := max(1, rows-1)
	v.detailOffset = min(v.detailOffset, max(0, len(wrapped)-available))
	end := min(len(wrapped), v.detailOffset+available)
	for _, part := range wrapped[v.detailOffset:end] {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func rawTerminal() (func(), error) { return tui.Raw(os.Stdin) }

func terminalSize() (int, int) { return tui.Size(os.Stdout) }

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
	if tui.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return runewidth.TruncateLeft(value, tui.Width(value)-width+1, "…")
}
func fit(value string, width int) string { return tui.Clip(value, width) }
func visibleWidth(value string) int {
	return tui.Width(value)
}
