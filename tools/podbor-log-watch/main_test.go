package main

import (
	"strings"
	"testing"
)

func TestStringListDeduplicatesPods(t *testing.T) {
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

func TestLogArgsAreReadOnly(t *testing.T) {
	cfg := config{
		kubeconfig: "/tmp/kubeconfig",
		context:    "example-context",
		namespace:  "example-namespace",
		tail:       50,
	}
	args := logArgs(cfg, "example-api-a")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, " logs -f pod/example-api-a ") {
		t.Fatalf("unexpected log args: %q", joined)
	}
	if strings.Contains(joined, "--timestamps") {
		t.Fatalf("kubectl timestamps waste horizontal space: %q", joined)
	}
	for _, mutation := range []string{"create", "apply", "patch", "delete", "rollout", "restart"} {
		if strings.Contains(joined, " "+mutation+" ") {
			t.Fatalf("mutation %q found in args: %q", mutation, joined)
		}
	}
}

func TestNormalizeKubectlLineRemovesDuplicatePodPrefix(t *testing.T) {
	container, body := normalizeKubectlLine(
		"example-api-aaa",
		"[pod/example-api-aaa/api] 2026-07-31 12:00:00 [info] request handled",
	)
	if container != "api" {
		t.Fatalf("unexpected container: %q", container)
	}
	if body != "2026-07-31 12:00:00 [info] request handled" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestPodAliasesUseUniqueSuffixes(t *testing.T) {
	got := podAliases([]string{
		"example-api-7d9f8-aaaaa",
		"example-api-7d9f8-bbbbb",
	})
	if got["example-api-7d9f8-aaaaa"] != "aaaaa" {
		t.Fatalf("unexpected first alias: %q", got["example-api-7d9f8-aaaaa"])
	}
	if got["example-api-7d9f8-bbbbb"] != "bbbbb" {
		t.Fatalf("unexpected second alias: %q", got["example-api-7d9f8-bbbbb"])
	}
}

func TestQueryTermsAndMatchesAll(t *testing.T) {
	terms := queryTerms(" ERROR  request-id error ")
	if len(terms) != 2 || terms[0] != "error" || terms[1] != "request-id" {
		t.Fatalf("unexpected terms: %#v", terms)
	}
	if !matchesAll("request-id=42 level=ERROR", terms) {
		t.Fatal("expected all terms to match")
	}
	if matchesAll("level=error", terms) {
		t.Fatal("expected missing request-id not to match")
	}
}

func TestHighlightMarksEveryTerm(t *testing.T) {
	got := highlight("ERROR request-id=42", []string{"error", "42"}, colorRed)
	if strings.Count(got, colorHighlight) != 2 {
		t.Fatalf("expected two highlights, got %q", got)
	}
	if !strings.Contains(got, colorRed) {
		t.Fatalf("expected severity color to be preserved, got %q", got)
	}
}

func TestHighlightSupportsUnicodeWords(t *testing.T) {
	got := highlight("Ошибка проверки анкеты", []string{"ошибка", "анкеты"}, colorRed)
	if strings.Count(got, colorHighlight) != 2 {
		t.Fatalf("expected two Unicode highlights, got %q", got)
	}
}

func TestImportantFieldsHaveSemanticColors(t *testing.T) {
	line := "duration=1.8878 execution_time_ms=11.09 method=GET status_code=200"
	got := highlight(line, nil, severityColor(line))
	for _, color := range []string{colorCyan, colorBlue, colorGreen} {
		if !strings.Contains(got, color) {
			t.Fatalf("expected color %q in %q", color, got)
		}
	}

	clientError := highlight("status_code=404", nil, "")
	if !strings.Contains(clientError, colorYellow) {
		t.Fatalf("expected 4xx to be yellow: %q", clientError)
	}
	serverError := highlight("status_code=503", nil, "")
	if !strings.Contains(serverError, colorRed) {
		t.Fatalf("expected 5xx to be red: %q", serverError)
	}
}

func TestQueryHighlightOverridesFieldColor(t *testing.T) {
	got := highlight("duration=1.8878", []string{"1.8878"}, "")
	if !strings.Contains(got, colorCyan) || !strings.Contains(got, colorHighlight) {
		t.Fatalf("expected field color and query highlight: %q", got)
	}
}

func TestSeverityUsesLevelMarkerNotMessageFields(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		level string
		color string
	}{
		{
			name:  "padded info with trace id",
			line:  "2026-07-31 12:36:31 [info     ] request_completed trace_id=abc",
			level: "info",
			color: "",
		},
		{
			name:  "trace id is not trace level",
			line:  "request_completed trace_id=abc",
			level: "",
			color: "",
		},
		{
			name:  "padded warning",
			line:  "2026-07-31 12:36:33 [warning  ] publication not found",
			level: "warning",
			color: colorYellow,
		},
		{
			name:  "structured debug",
			line:  `{"level":"debug","trace_id":"abc","message":"request"}`,
			level: "debug",
			color: colorDim,
		},
		{
			name:  "structured error",
			line:  `level=error trace_id=abc message="failed"`,
			level: "error",
			color: colorRed,
		},
		{
			name:  "python stack frame",
			line:  `File "/app/src/utils/transport.py", line 181, in get`,
			level: "stacktrace",
			color: colorDimRed,
		},
		{
			name:  "python stack code",
			line:  `    return await self._call_api("GET", path)`,
			level: "stacktrace",
			color: colorDimRed,
		},
		{
			name:  "python final exception",
			line:  `utils.transport.exceptions.HTTPError: HTTP 404: Request failed`,
			level: "error",
			color: colorRed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := logLevel(test.line); got != test.level {
				t.Fatalf("logLevel() = %q, want %q", got, test.level)
			}
			if got := severityColor(test.line); got != test.color {
				t.Fatalf("severityColor() = %q, want %q", got, test.color)
			}
		})
	}
}

func TestSanitizeRemovesTerminalControls(t *testing.T) {
	got := sanitize("safe\x1b[2J\tvalue\n")
	if got != "safe[2J value" {
		t.Fatalf("unexpected sanitized value: %q", got)
	}
}

func TestAppendBoundedKeepsNewestLines(t *testing.T) {
	got := []int{}
	for value := 1; value <= 5; value++ {
		got = appendBounded(got, value, 3)
	}
	want := []int{3, 4, 5}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPausedViewerKeepsFrozenSnapshotWhenNewLogsArrive(t *testing.T) {
	current := viewer{
		cfg:     config{buffer: 100},
		aliases: map[string]string{"example-api-a": "a"},
		lines:   []logLine{{pod: "example-api-a", text: "selected"}},
	}
	current.togglePause()
	current.addLine(logLine{pod: "example-api-a", text: "first"})
	current.addLine(logLine{pod: "example-api-a", text: "second"})

	visible := current.visibleLines("")
	if len(visible) != 1 || visible[0].text != "selected" {
		t.Fatalf("pause snapshot moved with new logs: %#v", visible)
	}
	if buffered := len(current.lines) - len(current.pauseBuffer); buffered != 2 {
		t.Fatalf("expected two separately buffered lines, got %d", buffered)
	}
}

func TestRenderListUsesCompactPodAlias(t *testing.T) {
	current := viewer{
		aliases: map[string]string{"example-api-7d9f8-aaaaa": "aaaaa"},
		width:   100,
	}
	line := logLine{
		pod:  "example-api-7d9f8-aaaaa",
		text: "2026-07-31 12:00:00 [info] request handled",
	}
	var out strings.Builder
	current.renderList(&out, []logLine{line}, 1, nil)
	got := out.String()
	if !strings.Contains(got, "aaaaa") {
		t.Fatalf("compact alias is missing: %q", got)
	}
	if strings.Contains(got, "example-api-7d9f8-aaaaa") {
		t.Fatalf("full pod name wastes list width: %q", got)
	}
}
