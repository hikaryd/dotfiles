package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"dots/native/internal/testpty"
	"dots/native/internal/tui"
)

func gatewayView(t *testing.T, count int) view {
	t.Helper()
	p := fixtureProfile()
	p.CustomOnly = true
	p.Queries = []metric{{ID: "edge-rps", Title: "Kong · запросы в секунду", Unit: "req/s", Section: "ingress", Query: "example_gateway_rps", Scope: "сервисы example"}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	v := newView(p, time.Hour, time.Minute)
	now := time.Now()
	end := float64(now.Unix())
	data := snapshot{Metric: "edge-rps", Unit: "req/s", Start: end - 3600, End: end, Step: 60, FetchedAt: now}
	for i := 0; i < count; i++ {
		value := float64((i + 1) * 10)
		s := series{Labels: map[string]string{"service": fmt.Sprintf("service-%02d", i)}, Current: &point{At: end, Value: &value}}
		for j := 0; j <= 60; j++ {
			n := value * float64(j) / 60
			s.Points = append(s.Points, point{At: data.Start + float64(j*60), Value: &n})
		}
		data.Series = append(data.Series, s)
	}
	v.apply("edge-rps", data, nil)
	return v
}

func TestKongOverviewShowsEverySourceCurrentAndHistory(t *testing.T) {
	v := gatewayView(t, 2)
	out := v.frame(220, 35, false).Render()
	if strings.Contains(out, "групп →") {
		t.Fatal("multiple sources replaced by a group count")
	}
	for _, want := range []string{"service-00", "service-01", "10 запр./с", "20 запр./с", "Сервис / источник"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s:\n%s", want, out)
		}
	}
	for _, name := range []string{"service-00", "service-01"} {
		drawn := false
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, name) && strings.ContainsAny(line, "▁▂▃▄▅▆▇█") {
				drawn = true
			}
		}
		if !drawn {
			t.Fatalf("%s has no own history:\n%s", name, out)
		}
	}
}

func TestOverviewSourceSelectionSurvivesDetailAndRefresh(t *testing.T) {
	v := gatewayView(t, 2)
	v.moveOverview(1) // source 00 is lower-ranked initially
	if v.seriesIndex != 1 {
		t.Fatal("arrows did not navigate individual sources")
	}
	v.detail = true
	if ranked(v.readings[v.focused().ID].Data)[v.seriesIndex].Labels["service"] != "service-00" {
		t.Fatal("Enter would open a different source")
	}
	v.detail = false
	data := *v.readings["edge-rps"].Data
	data.Series = append([]series{}, data.Series...)
	n := 99.0
	data.Series[0].Current = &point{At: data.End, Value: &n}
	v.apply("edge-rps", data, nil)
	rows := v.overviewRows()
	row := rows[v.overviewIndex(rows)]
	if row.Source.Labels["service"] != "service-00" {
		t.Fatal("refresh moved selection to another source")
	}
}

func TestOverviewKeepsHistoryButNeverSubstitutesMissingCurrent(t *testing.T) {
	v := gatewayView(t, 2)
	v.readings["edge-rps"].Data.Series[0].Current = nil
	for _, row := range v.overviewRows() {
		if row.Source.Labels["service"] == "service-00" {
			if singlePoint(row.Reading) != nil || v.state(row.Reading) != "undefined" {
				t.Fatal("history was substituted as current, or another source hid missing data")
			}
		}
	}
	out := v.frame(220, 35, false).Render()
	if !strings.Contains(out, "нет данных") || !strings.Contains(out, "нет значения") {
		t.Fatal(out)
	}
}

func TestMultiSourceLiveEnterKeepsSourceAndModalOwnsFocus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		kind := "vector"
		var rows []any
		for i, name := range []string{"example-api", "example-worker"} {
			value := float64((i + 1) * 10)
			row := map[string]any{"metric": map[string]string{"service": name}}
			if strings.HasSuffix(r.URL.Path, "query_range") {
				kind = "matrix"
				start, _ := strconv.ParseFloat(r.Form.Get("start"), 64)
				end, _ := strconv.ParseFloat(r.Form.Get("end"), 64)
				step, _ := strconv.ParseFloat(r.Form.Get("step"), 64)
				var points [][]any
				for at := start; at <= end; at += step {
					points = append(points, []any{at, fmt.Sprint(value * (at - start) / (end - start))})
				}
				row["values"] = points
			} else {
				at, _ := strconv.ParseFloat(r.Form.Get("time"), 64)
				row["value"] = []any{at, fmt.Sprint(value)}
			}
			rows = append(rows, row)
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": kind, "result": rows}})
	}))
	defer server.Close()
	v := gatewayView(t, 2)
	p := v.profile
	p.URL = server.URL
	terminal := testpty.Start(t, cli(t, "--config", saveConfig(t, p)))
	terminal.Resize(160, 32)
	terminal.Screen("Получены данные: 1 из 1 · рядов: 2")
	terminal.Send("j\r")
	terminal.Screen("Сейчас  10 запр./с")
	terminal.Screen("2/2  service=example-api")
	offset := len(terminal.Text())
	terminal.Send("k")
	terminal.After(offset, "Сейчас  20 запр./с")
	offset = len(terminal.Text())
	terminal.Send("j")
	terminal.After(offset, "Сейчас  10 запр./с")
	offset = len(terminal.Text())
	terminal.Send("\x1b")
	terminal.After(offset, "Получены данные: 1 из 1")
	terminal.Send("\t")
	terminal.Screen("РАЗДЕЛЫ · ↑↓/jk Enter")
	offset = len(terminal.Text())
	terminal.Send("k")
	terminal.After(offset, "› Очереди")
	offset = len(terminal.Text())
	terminal.Send("j")
	terminal.After(offset, "› Вход и доступность")
	terminal.Send("s")
	terminal.Screen("РАЗДЕЛЫ · ↑↓/jk")
	terminal.Send("k\r")
	terminal.Screen("Нет настроенных показателей")
	terminal.Send("q")
	terminal.Wait(0)
}

func TestExpandedSourceTableFitsAndScrollsAtEveryWidth(t *testing.T) {
	for _, size := range [][2]int{{40, 16}, {60, 20}, {80, 24}, {100, 28}, {120, 30}, {160, 35}, {280, 50}} {
		v := gatewayView(t, 16)
		v.seriesIndex = 15
		for _, nav := range []bool{false, true} {
			v.navFocus = nav
			v.navCursor = v.section
			out := v.frame(size[0], size[1], false).Render()
			if strings.Contains(out, "More details") {
				t.Fatalf("overflow %v:\n%s", size, out)
			}
			if !nav && !strings.Contains(out, "› ") {
				t.Fatalf("selected source scrolled away: %v", size)
			}
			for _, line := range strings.Split(out, "\n") {
				if tui.Width(line) > size[0] {
					t.Fatal("width overflow")
				}
			}
		}
	}
}
