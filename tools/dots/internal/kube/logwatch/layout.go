package logwatch

import (
	"strings"

	"dots/native/internal/kube/tui"
)

type logColumns struct{ pod, container, message int }

// Use full identifiers whenever they fit. Reserve message space first, then
// shorten metadata only on genuinely narrow screens, not at arbitrary 8/10 cells.
func (v *viewer) columns(lines []logLine) logColumns {
	podWidth, containerWidth := 3, 9
	for _, pod := range v.cfg.pods {
		podWidth = max(podWidth, tui.Width(pod))
	}
	for _, line := range lines {
		containerWidth = max(containerWidth, tui.Width(line.container))
	}
	const fixed = 27 // marker, received time, level, and four separators
	available := max(12, v.width-fixed-max(24, v.width/3))
	if podWidth+containerWidth > available {
		containerWidth = min(containerWidth, max(6, available/3))
		podWidth = min(podWidth, max(6, available-containerWidth))
	}
	return logColumns{podWidth, containerWidth, max(1, v.width-fixed-podWidth-containerWidth)}
}
func (c logColumns) header() string {
	return "  " + tui.Pad("RECEIVED", 8) + " │ " + tui.Pad("POD", c.pod) + " │ " + tui.Pad("CONTAINER", c.container) + " │ " + tui.Pad("LEVEL", 5) + " │ MESSAGE"
}
func (v *viewer) podTone(pod string) string {
	tones := []string{colorCyan, colorGreen, colorMagenta, colorYellow, tui.Text}
	for index, name := range v.cfg.pods {
		if name == pod {
			return tones[index%len(tones)]
		}
	}
	return tui.Text
}
func levelLabel(text string) string {
	switch logLevel(text) {
	case "fatal", "critical", "panic":
		return "FATAL"
	case "error", "err", "exception", "traceback":
		return "ERROR"
	case "warning", "warn":
		return "WARN"
	case "info", "notice":
		return "INFO"
	case "debug":
		return "DEBUG"
	case "trace":
		return "TRACE"
	default:
		return "—"
	}
}
func (v *viewer) recordPrefix(line logLine, marker string, c logColumns) string {
	level := levelLabel(line.text)
	tone := severityColor(line.text)
	if level == "INFO" {
		tone = colorGreen
	}
	separator := colorDim + " │ " + colorReset
	return colorDim + marker + " " + line.at.Format("15:04:05") + colorReset + separator +
		v.podTone(line.pod) + tui.Pad(compact(line.pod, c.pod), c.pod) + colorReset + separator +
		colorDim + tui.Pad(compact(firstNonEmpty(line.container, "—"), c.container), c.container) + colorReset + separator +
		tone + tui.Pad(level, 5) + colorReset + separator
}
func continuation(prefix string) string { return strings.Repeat(" ", tui.Width(prefix)) }
