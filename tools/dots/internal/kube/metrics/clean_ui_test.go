package metrics

import (
	"strings"
	"testing"
	"time"

	"dots/native/internal/tui"
)

func TestOverviewRemovesRepeatedChromeButKeepsNotesInHelp(t *testing.T) {
	v := gatewayView(t, 8)
	v.profile.SectionNotes = map[string]string{"ingress": "Длинное пояснение о покрытии источников"}
	out := v.frame(240, 35, false).Render()
	if strings.Count(out, "Kong · запросы в секунду") != 1 {
		t.Fatal("metric name repeated on every source row")
	}
	if strings.Count(out, "с назад") > 1 {
		t.Fatal("same refresh age repeated in every row")
	}
	for _, noise := range []string{"Длинное пояснение", "Каждая строка —", "без изменений"} {
		if strings.Contains(out, noise) {
			t.Fatalf("overview noise: %s", noise)
		}
	}
	if !strings.Contains(strings.Join(v.helpLines(120), "\n"), "Длинное пояснение") {
		t.Fatal("source caveat deleted instead of moved to help")
	}
	v.readings["edge-rps"] = reading{Data: v.readings["edge-rps"].Data, Error: "test"}
	out = v.frame(240, 35, false).Render()
	if !strings.Contains(out, "старые данные") {
		t.Fatal("failed refresh hidden with redundant age")
	}
}

func TestRouteMethodsHaveIndependentColorsAndReadableIdentities(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.chooseSection("routes")
	v.profile.RouteLabel = "endpoint"
	for _, m := range v.rows() {
		for i := range v.readings[m.ID].Data.Series {
			labels := v.readings[m.ID].Data.Series[i].Labels
			labels["endpoint"] = labels["handler"]
			delete(labels, "handler")
		}
	}
	tones := []string{tui.Green, tui.Accent, tui.Yellow, tui.Purple, tui.Red}
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	base := v.readings["routerps"].Data.Series[0]
	var seriesRows []series
	for _, method := range methods {
		s := base
		s.Labels = map[string]string{"endpoint": "/items/{id}", "method": method, "service": "example-api"}
		seriesRows = append(seriesRows, s)
	}
	v.readings["routerps"].Data.Series = seriesRows
	for _, width := range []int{80, 120, 180, 260} {
		out := v.frame(width, 40, true).Render()
		if strings.Contains(out, "endpoint=") || strings.Contains(out, "method=") {
			t.Fatal("raw labels leaked into overview")
		}
		if !strings.Contains(out, "/items/{id}") || !strings.Contains(out, "example-api") {
			t.Fatalf("route or owner lost at %d", width)
		}
		for i, method := range methods {
			if !strings.Contains(out, tones[i]+tui.Bold+method) {
				t.Fatalf("missing %s color at %d", method, width)
			}
		}
		plain := v.frame(width, 40, false).Render()
		if strings.Contains(plain, "\x1b") {
			t.Fatal("NO_COLOR output has escape codes")
		}
	}
}

func TestEmptyCustomAndRuntimeAreNotNavigationDestinations(t *testing.T) {
	p := fixtureProfile()
	p.validate()
	v := newView(p, time.Hour, time.Minute)
	for _, out := range []string{strings.Join(v.sidebar(30, false), "\n"), strings.Join(v.sectionMenu(100, 30), "\n")} {
		if strings.Contains(out, "Свои") || strings.Contains(out, "Runtime") || strings.Contains(out, "Исходящие вызовы") {
			t.Fatal("unused sections clutter navigation")
		}
	}
	v.sectionCursor, _ = sectionIndex("health")
	v.sectionKey("text", "j")
	if sections[v.sectionCursor].ID != "routes" {
		t.Fatal("j did not skip hidden custom section")
	}
	v.sectionKey("text", "k")
	if sections[v.sectionCursor].ID != "health" {
		t.Fatal("k did not move back")
	}
	v.sectionCursor, _ = sectionIndex("queues")
	v.sectionKey("text", "j")
	if sections[v.sectionCursor].ID != "ingress" {
		t.Fatal("empty outbound is still a navigation destination")
	}
	for _, kind := range []string{"metrics", "services", "route"} {
		v.openPicker(kind)
		v.pickerKey("text", "j")
		v.pickerKey("text", "k")
		if v.picker.Query != "jk" {
			t.Fatalf("j/k hijacked %s text input", kind)
		}
	}
}
