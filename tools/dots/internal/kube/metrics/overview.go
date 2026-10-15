package metrics

import (
	"fmt"
	"time"

	"dots/native/internal/tui"
)

// A visible row is one source of one metric. Never add unrelated services,
// percentile estimates, percentages or Redis replicas just to draw a trend.
type overviewRow struct {
	Metric                   metric
	MetricIndex, SeriesIndex int
	Source                   *series
	Reading                  reading
}

func (v view) overviewRows() []overviewRow {
	var rows []overviewRow
	for mi, m := range v.rows() {
		r := v.readings[m.ID]
		sources := ranked(r.Data)
		if len(sources) == 0 {
			rows = append(rows, overviewRow{Metric: m, MetricIndex: mi, Reading: r})
			continue
		}
		for si, s := range sources {
			copy := *r.Data
			copy.Series = []series{s}
			one := r
			one.Data = &copy
			rows = append(rows, overviewRow{Metric: m, MetricIndex: mi, SeriesIndex: si, Source: &s, Reading: one})
		}
	}
	return rows
}

func (v view) overviewIndex(rows []overviewRow) int {
	found := 0
	for i, row := range rows {
		if row.MetricIndex == v.selected {
			found = i
			if row.SeriesIndex == v.seriesIndex {
				return i
			}
		}
	}
	return found
}

func (v *view) moveOverview(delta int) {
	rows := v.overviewRows()
	if len(rows) == 0 {
		return
	}
	at := (v.overviewIndex(rows) + delta + len(rows)) % len(rows)
	v.selected, v.seriesIndex = rows[at].MetricIndex, rows[at].SeriesIndex
}

func sourceName(labels map[string]string) string {
	if len(labels) == 0 {
		return "вместе"
	}
	if len(labels) == 1 {
		for _, value := range labels {
			return tui.Clean(value)
		}
	}
	return tui.Clean(seriesLabel(labels))
}

func (v view) overviewLines(width, budget int, color bool) []string {
	metrics := v.rows()
	if len(metrics) == 0 {
		return v.emptySection(width, budget)
	}
	rows := v.overviewRows()
	selected := v.overviewIndex(rows)
	ready, missing, failed := 0, 0, 0
	hasSources := false
	var oldest time.Time
	for _, m := range metrics {
		r := v.readings[m.ID]
		if r.Data != nil {
			if len(r.Data.Series) > 1 || len(r.Data.Series) == 1 && len(r.Data.Series[0].Labels) > 0 {
				hasSources = true
			}
			if oldest.IsZero() || r.Data.FetchedAt.Before(oldest) {
				oldest = r.Data.FetchedAt
			}
		}
		switch v.state(r) {
		case "ready", "paused", "bounded":
			ready++
		case "empty", "undefined":
			missing++
		case "error", "stale":
			failed++
		}
	}
	coverage := fmt.Sprintf("Получены данные: %d из %d", ready, len(metrics))
	if hasSources {
		coverage += fmt.Sprintf(" · рядов: %d", len(rows))
	}
	if missing > 0 {
		coverage += fmt.Sprintf(" · без измерений: %d", missing)
	}
	if failed > 0 {
		coverage += fmt.Sprintf(" · ошибки запросов: %d", failed)
	}
	if !oldest.IsZero() {
		coverage += " · обновлено " + durationText(v.now.Sub(oldest)) + " назад"
	}
	lines := []string{tui.Paint(tui.Clip(coverage, width), tui.Dim, color)}
	if text := v.observation(); text != "" {
		lines = append(lines, tui.Clip(text, width))
	}
	routes := sections[v.section].ID == "routes" && v.profile.RouteLabel != ""
	compact := width >= 76 && (!hasSources || width >= 100) && (!routes || width >= 140)
	nameWidth, valueWidth, trendWidth, statusWidth, changeWidth, ownerWidth := 28, 22, 26, 14, 0, 0
	if width < 130 {
		nameWidth = 24
		valueWidth = 20
		trendWidth = 18
	}
	if width >= 160 {
		changeWidth = 13
	}
	if routes {
		nameWidth = 13
		valueWidth = 19
	}
	if routes && width >= 140 {
		ownerWidth = min(30, width/6)
	}
	sourceWidth := 0
	if hasSources {
		sourceWidth = width - nameWidth - valueWidth - trendWidth - statusWidth - changeWidth - ownerWidth
	} else {
		trendWidth = min(40, max(8, width-nameWidth-valueWidth-statusWidth-changeWidth))
	}
	cell := func(s string, w int) string { return tui.Pad(tui.Clip(s, max(1, w-1)), w) }
	if compact {
		header := cell("  Показатель", nameWidth)
		if hasSources {
			label := "Сервис / источник"
			if routes {
				label = "Метод    Маршрут"
			}
			header += cell(label, sourceWidth)
		}
		if ownerWidth > 0 {
			header += cell("Сервис", ownerWidth)
		}
		header += cell("Значение", valueWidth) + cell("Динамика", trendWidth)
		if changeWidth > 0 {
			header += cell("Δ к началу", changeWidth)
		}
		lines = append(lines, tui.Paint(header, tui.Dim, color))
	}
	lineCount := 1
	if !compact {
		lineCount = 2
	}
	capacity := max(1, (budget-len(lines)-1)/lineCount)
	start := max(0, selected-capacity+1)
	end := min(len(rows), start+capacity)
	for i := start; i < end; i++ {
		row := rows[i]
		m, r := row.Metric, row.Reading
		active := i == selected && !v.navFocus
		mark := "  "
		if active {
			mark = "› "
		}
		name := ""
		if i == start || rows[i-1].MetricIndex != row.MetricIndex {
			name = v.overviewTitle(m)
		}
		value, trend, change := pointText(singlePoint(r), m.Unit), "—", ""
		labels := map[string]string{}
		if row.Source != nil {
			labels = row.Source.Labels
			trend = spark(row.Source.Points, r.Data.Start, r.Data.End, trendWidth-1)
			if v.state(r) == "ready" || v.state(r) == "paused" {
				change = compactChange(*row.Source, r.Data, m.Unit)
			}
		}
		if id := companions[m.ID]; id != "" && !v.profile.CustomOnly {
			pair := v.readings[id]
			if single(pair) != nil && v.state(pair) == "ready" {
				value += " · " + number(single(pair)) + "/с"
			}
		}
		status := ""
		if state := v.state(r); state != "ready" && state != "paused" {
			status = v.status(r)
		}
		tone, trendTone := tui.Text, tui.Dim
		if active {
			tone = tui.Accent + tui.Bold
			trendTone = tui.Accent
		}
		valueTone := tone
		if v.state(r) == "bounded" || v.state(r) == "old" {
			valueTone = tui.Yellow
		}
		if v.state(r) == "stale" || v.state(r) == "error" {
			valueTone = tui.Red
		}
		if compact {
			nameTone := tone
			if name != "" {
				nameTone += tui.Bold
			}
			line := tui.Paint(cell(mark+name, nameWidth), nameTone, color)
			if hasSources {
				line += v.sourceCell(labels, sourceWidth, color, active, false)
			}
			if ownerWidth > 0 {
				line += tui.Paint(cell(tui.Clean(labels["service"]), ownerWidth), tui.Dim, color)
			}
			line += tui.Paint(cell(value, valueWidth), valueTone, color) + tui.Paint(cell(trend, trendWidth), trendTone, color)
			if changeWidth > 0 {
				line += tui.Paint(cell(change, changeWidth), tui.Dim, color)
			}
			line += tui.Paint(tui.Clip(status, statusWidth), v.statusTone(r), color)
			lines = append(lines, line)
		} else {
			prefix := mark
			if name != "" {
				prefix += name
				if hasSources {
					prefix += " · "
				}
			}
			// Keep room for route identity, even when the title is long.
			if hasSources {
				prefix = tui.Clip(prefix, max(2, width/3)) + " "
			} else {
				prefix = tui.Clip(prefix, width)
			}
			label := tui.Paint(prefix, tone, color)
			if hasSources {
				label += v.sourceCell(labels, width-tui.Width(prefix), color, active, false)
			}
			lines = append(lines, tui.Clip(label, width))
			sparkWidth := min(22, max(5, width/4))
			if row.Source != nil {
				trend = spark(row.Source.Points, r.Data.Start, r.Data.End, sparkWidth-1)
			}
			valuesWidth := min(22, max(16, width-sparkWidth-statusWidth-2))
			line := tui.Paint(cell("  "+value, valuesWidth), valueTone, color) + tui.Paint(cell(trend, sparkWidth), trendTone, color)
			if routes {
				ownerSpace := max(0, width-valuesWidth-sparkWidth-tui.Width(status)-1)
				line += tui.Paint(cell(tui.Clean(labels["service"]), ownerSpace), tui.Dim, color)
			}
			line += tui.Paint(status, v.statusTone(r), color)
			lines = append(lines, tui.Clip(line, width))
		}
	}
	context := v.scopeLabel(rows[selected].Metric.ID)
	if routes && ownerWidth == 0 && rows[selected].Source != nil {
		context = tui.Clean(rows[selected].Source.Labels["service"]) + " · " + context
	}
	lines = append(lines, tui.Paint(tui.Clip(fmt.Sprintf("%d–%d из %d · %s", start+1, end, len(rows), context), width), tui.Dim, color))
	return lines
}
