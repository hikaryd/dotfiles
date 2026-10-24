package metrics

import (
	"strings"
	"testing"
	"time"

	"dots/native/internal/tui"
)

func TestDatabaseNavigationDoesNotDuplicateMetricChoices(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.chooseSection("db")
	sidebar := strings.Join(v.sidebar(40, false), "\n")
	for _, m := range v.rows() {
		if strings.Contains(sidebar, m.Title) {
			t.Fatalf("sidebar duplicates the metric table: %s", m.Title)
		}
	}
	if !strings.Contains(sidebar, "Пулы и другие БД") {
		t.Fatal("section navigation disappeared")
	}
}

func TestOnlyFocusedPanelShowsSelectionCursor(t *testing.T) {
	for _, focus := range []bool{false, true} {
		v := preview("normal", time.Hour, time.Minute)
		v.chooseSection("db")
		v.selected = 1
		v.navFocus = focus
		v.navCursor = v.section
		out := tui.Plain(v.frame(160, 35, true).Render())
		if n := strings.Count(out, "› "); n != 1 {
			t.Fatalf("sidebar focus=%v: want one selection cursor, got %d", focus, n)
		}
	}
}
