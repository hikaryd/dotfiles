package metrics

import (
	"errors"
	"strings"
	"testing"
	"time"

	"dots/native/internal/tui"
)

func TestOverviewAndDetailLayoutsRemainInsideTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {100, 28}, {80, 24}, {60, 20}, {40, 16}} {
		for _, scenario := range []string{"normal", "degraded", "missing", "error"} {
			v := preview(scenario, time.Hour, time.Minute)
			for section := range 4 {
				v.section = section
				v.selected = len(v.rows()) - 1
				frame := v.frame(size[0], size[1], false)
				out := frame.Render()
				if strings.Contains(out, "More details are available") {
					t.Fatalf("hidden selection at %v: %s", size, out)
				}
				if !strings.Contains(out, "› ") || !strings.Contains(out, "q выход") {
					t.Fatalf("selection or exit lost: %v\n%s", size, out)
				}
				lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
				if len(lines) > size[1] {
					t.Fatalf("height overflow %v: %d", size, len(lines))
				}
				for _, line := range lines {
					if tui.Width(line) > size[0] {
						t.Fatalf("width overflow %v: %s", size, line)
					}
				}
			}
		}
	}
}

func TestDetailLayoutsAndHelpStayNavigable(t *testing.T) {
	for _, size := range [][2]int{{100, 28}, {80, 24}, {40, 16}} {
		for _, scenario := range []string{"normal", "missing", "error"} {
			for _, id := range []string{"errors5xx", "services", "cpulimits"} {
				v := preview(scenario, time.Hour, time.Minute)
				v.chooseMetric(id)
				out := v.frame(size[0], size[1], false).Render()
				if strings.Contains(out, "More details are available") || !strings.Contains(out, "q выход") {
					t.Fatalf("detail overflow %v %s:\n%s", size, id, out)
				}
			}
		}
	}
	query := "metric{label=\"" + strings.Repeat("a", 120) + "  b\"}"
	if strings.Join(wrap(query, 40), "") != query {
		t.Fatal("help changed PromQL whitespace")
	}
}

func TestMeaningfulStatesAndNoMadeUpZero(t *testing.T) {
	v := preview("missing", time.Hour, time.Minute)
	if v.state(v.readings["p95"]) != "empty" {
		t.Fatal("missing not explicit")
	}
	if v.state(v.readings["restarts"]) != "ready" || *single(v.readings["restarts"]) != 0 {
		t.Fatal("zero confused with missing")
	}
	r := v.readings["rps"]
	r.Data.Series[0].Current.Value = nil
	if v.state(r) != "undefined" {
		t.Fatal("NaN not explicit")
	}
	r.Error = "request failed"
	if v.state(r) != "stale" {
		t.Fatal("old data presented as live")
	}
	v.apply("rps", snapshot{}, errors.New("failed"))
	if v.readings["rps"].Data == nil || v.readings["rps"].Error == "" {
		t.Fatal("last good data was lost")
	}
	if v.state(reading{Error: "failed"}) != "error" {
		t.Fatal("initial failure not explicit")
	}
	v.paused = true
	if v.state(v.readings["restarts"]) != "paused" {
		t.Fatal("pause not explicit")
	}
}

func TestRankAndSelectionStayAttachedToLabels(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.chooseMetric("services")
	before := ranked(v.readings["services"].Data)
	v.seriesIndex = 2
	selected := labelKey(before[2].Labels)
	data := *v.readings["services"].Data
	data.Series = append([]series{}, data.Series...)
	n := 999.0
	data.Series[0].Current = &point{At: data.End, Value: &n}
	v.apply("services", data, nil)
	after := ranked(v.readings["services"].Data)
	if labelKey(after[v.seriesIndex].Labels) != selected {
		t.Fatal("refresh moved cursor to a different service")
	}
	if *current(after[0]) != 999 {
		t.Fatal("not ranked by value")
	}
}

func TestScopeAndQueryPlanAreBoundedAndLazy(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	if len(v.plan()) != 9 {
		t.Fatalf("overview plan has %d queries", len(v.plan()))
	}
	v.detail = true
	if len(v.plan()) != 1 || v.plan()[0].ID != "rps" {
		t.Fatal("detail queries entire catalog")
	}
	v.breakdown = true
	if v.plan()[0].ID != "services" {
		t.Fatal("wrong breakdown")
	}
	if effectiveStep(24*time.Hour, time.Second) != time.Minute {
		t.Fatal("24h point bound wrong")
	}
	v.section = 4
	v.detail = false
	v.breakdown = false
	if len(v.plan()) != 0 {
		t.Fatal("empty custom section should not query")
	}
}

func TestCustomOnlyDoesNotInheritBuiltinSemantics(t *testing.T) {
	p := fixtureProfile()
	p.CustomOnly = true
	p.Queries = []metric{{ID: "errors5xx", Title: "Custom counter", Unit: "count", Query: "up"}}
	p.validate()
	v := newView(p, time.Hour, time.Minute)
	if len(v.plan()) != 1 || v.scopeLabel("errors5xx") != "область задана запросом" {
		t.Fatal("custom query inherited builtin semantics")
	}
	v.chooseMetric("errors5xx")
	if v.breakdown {
		t.Fatal("unexpected builtin breakdown")
	}
}

func TestChartsKeepMissingBucketsAndDoNotConnectGaps(t *testing.T) {
	a, b := 2.0, 4.0
	points := []point{{At: 0, Value: &a}, {At: 2, Value: nil}, {At: 4, Value: &b}}
	out := spark(points, 0, 4, 5)
	if []rune(out)[2] != '·' || []rune(out)[1] != '·' {
		t.Fatalf("missing gaps interpolated: %s", out)
	}
	for _, data := range [][]point{points, {{At: 0, Value: &a}, {At: 4, Value: &b}}} {
		grid := lineGrid(data, 0, 4, 1, 41, 8, 0, 5)
		for _, row := range grid {
			for x := 1; x < 40; x++ {
				if row[x] != 0 {
					t.Fatal("connected across a null or absent sample")
				}
			}
		}
	}
	grid := lineGrid([]point{{At: 0, Value: &a}, {At: 1, Value: &b}}, 0, 1, 1, 41, 8, 0, 5)
	for x := range 41 {
		found := false
		for _, row := range grid {
			if row[x] != 0 {
				found = true
			}
		}
		if !found {
			t.Fatalf("adjacent real samples have disconnected column %d", x)
		}
	}
}

func TestDownsamplingRetainsBriefSpikesAndDrops(t *testing.T) {
	low, high := 1.0, 99.0
	points := []point{{At: 0, Value: &low}, {At: 1, Value: &high}, {At: 2, Value: &low}, {At: 10, Value: &low}}
	buckets := plotBuckets(points, 0, 10, 2)
	if buckets[0].High != 99 || buckets[0].Low != 1 {
		t.Fatal("downsampling erased the transient")
	}
	if !strings.HasPrefix(spark(points, 0, 10, 2), "█") {
		t.Fatal("sparkline hid the peak")
	}
}

func TestTerminalPayloadsAreCleaned(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.items[0].Title = "evil\x1b[2Jtitle"
	v.profile.Namespace = "test\x1b[2J"
	out := v.frame(100, 28, false).Render()
	if strings.Contains(out, "\x1b") {
		t.Fatalf("escape injection: %q", out)
	}
}
