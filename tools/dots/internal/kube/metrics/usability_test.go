package metrics

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"dots/native/internal/tui"
)

// Reproduces the screenshots: 16 minute samples spread across a wide, tall TTY.
// Empty screen columns are NOT missing data between adjacent API samples.
func TestWideRPSDetailShowsContinuousHistoryAndUsesHeight(t *testing.T) {
	v := preview("normal", 15*time.Minute, time.Minute)
	v.chooseMetric("rps")
	out := tui.Plain(v.frame(280, 50, false).Render())
	var grid []string
	for _, line := range strings.Split(out, "\n") {
		if _, right, ok := strings.Cut(line, "│"); ok {
			grid = append(grid, right)
		}
	}
	if len(grid) < 20 {
		t.Fatalf("graph still squashed into %d rows on a 50-row terminal", len(grid))
	}
	occupied := map[int]bool{}
	for _, line := range grid {
		for x, r := range []rune(line) {
			if strings.ContainsRune("─━╭╮╰╯┌┐└┘┼┤├┬┴", r) {
				occupied[x] = true
			}
		}
	}
	if len(occupied) < 200 {
		t.Fatalf("history is isolated samples, not a line: only %d connected columns", len(occupied))
	}
}

func TestRPSDetailExplainsNumbersInsteadOfInternalJargon(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.chooseMetric("rps")
	out := v.frame(100, 28, false).Render()
	for _, want := range []string{"Сейчас", "запр./с", "минимум", "пик"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing meaningful label %q", want)
		}
	}
	for _, jargon := range []string{"HTTP scope", "Series 1/1: total", "fetched", "min/max history", "timestamps local"} {
		if strings.Contains(out, jargon) {
			t.Errorf("implementation jargon on primary screen: %q", jargon)
		}
	}
}

func TestAxesHaveIntermediateTimeLabelsAndLegibleUnits(t *testing.T) {
	_, labels := timeAxis(1700000000, 1700000900, 100)
	if len(regexp.MustCompile(`[0-9]{2}:[0-9]{2}`).FindAllString(labels, -1)) < 4 {
		t.Fatal("no intermediate clock labels")
	}
	n := 1820.0
	if got := valueText(&n, "MiB"); got != "1,78 ГиБ" {
		t.Fatalf("memory formatting: %s", got)
	}
	n = 25200
	if got := number(&n); got != "25\u202f200" {
		t.Fatalf("count formatting: %s", got)
	}
	n = 1250
	if got := valueText(&n, "ms"); got != "1,25 с" {
		t.Fatalf("latency formatting: %s", got)
	}
}

func TestHumanNumbersDoNotUseScientificNotation(t *testing.T) {
	for _, value := range []float64{25200, 182000, 0.000012, 1000000} {
		out := number(&value)
		if strings.ContainsAny(out, "eE") {
			t.Errorf("%v displayed as %s", value, out)
		}
	}
}
