package metrics

import (
	"strings"

	"dots/native/internal/tui"
)

func methodTone(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return tui.Green
	case "POST":
		return tui.Accent
	case "PUT":
		return tui.Yellow
	case "PATCH":
		return tui.Purple
	case "DELETE":
		return tui.Red
	default:
		return tui.Dim
	}
}

// Format labels without changing their identity, grouping, or queries. Clip
// individual fields before painting: clipping an ANSI line drops its colors.
func (v view) sourceCell(labels map[string]string, width int, color, active, owner bool) string {
	tone := tui.Text
	if active {
		tone += tui.Bold
	}
	path := labels[v.profile.RouteLabel]
	if v.profile.RouteLabel == "" || path == "" {
		return tui.Paint(tui.Pad(tui.Clip(sourceName(labels), max(1, width-1)), width), tone, color)
	}
	method := tui.Clean(labels["method"])
	if method == "" {
		method = "—"
	}
	methodWidth := 9
	path = tui.Clean(path)
	if owner && labels["service"] != "" {
		path += " · " + tui.Clean(labels["service"])
	}
	return tui.Paint(tui.Pad(tui.Clip(method, methodWidth-1), methodWidth), methodTone(method)+tui.Bold, color) +
		tui.Paint(tui.Pad(tui.Clip(path, max(1, width-methodWidth-1)), max(0, width-methodWidth)), tone, color)
}

func (v view) overviewTitle(m metric) string {
	if sections[v.section].ID == "routes" && !v.profile.CustomOnly {
		if title := map[string]string{"routerps": "RPS", "routep95": "p95", "routep99": "p99", "routeerrors": "5xx · %", "routeerrorsrate": "5xx / с"}[m.ID]; title != "" {
			return title
		}
	}
	return v.displayMetricTitle(m)
}

func compactChange(s series, data *snapshot, unit string) string {
	text := strings.TrimSuffix(changeText(s, data, unit), " к началу")
	if text == "без изменений" {
		return "·"
	}
	return text
}
