package metrics

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"dots/native/internal/tui"
)

// Human output never uses scientific notation. Machine-readable JSON keeps the
// original values and units; localized formatting is presentation only.
func number(value *float64) string {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return "нет данных"
	}
	precision := 2
	if n := math.Abs(*value); n > 0 && n < 0.01 {
		precision = min(324, int(math.Ceil(-math.Log10(n)))+1)
	}
	raw := strconv.FormatFloat(*value, 'f', precision, 64)
	raw = strings.TrimRight(strings.TrimRight(raw, "0"), ".")
	if raw == "-0" {
		raw = "0"
	}
	parts := strings.SplitN(raw, ".", 2)
	integer := parts[0]
	sign := ""
	if strings.HasPrefix(integer, "-") {
		sign = "−"
		integer = integer[1:]
	}
	for i := len(integer) - 3; i > 0; i -= 3 {
		integer = integer[:i] + "\u202f" + integer[i:]
	}
	result := sign + integer
	if len(parts) == 2 {
		result += "," + parts[1]
	}
	return result
}

type displayScale struct {
	divisor float64
	unit    string
}

func scaleFor(unit string, magnitude float64) displayScale {
	s := displayScale{1, tui.Clean(unit)}
	switch unit {
	case "req/s":
		s.unit = "запр./с"
	case "req/min":
		s.unit = "запр./мин"
	case "ops/s":
		s.unit = "оп./с"
	case "jobs/s":
		s.unit = "задач/с"
	case "jobs":
		s.unit = "задач"
	case "connections":
		s.unit = "соед."
	case "clients":
		s.unit = "клиентов"
	case "days":
		s.unit = "дн."
	case "s":
		s.unit = "с"
		if magnitude > 0 && magnitude < 1 {
			s = displayScale{0.001, "мс"}
		}
	case "cores":
		s.unit = "ядра"
	case "ms":
		s.unit = "мс"
		if magnitude >= 1000 {
			s = displayScale{1000, "с"}
		}
	case "MiB":
		s.unit = "МиБ"
		if magnitude >= 1024*1024 {
			s = displayScale{1024 * 1024, "ТиБ"}
		} else if magnitude >= 1024 {
			s = displayScale{1024, "ГиБ"}
		}
	case "restarts":
		s.unit = "перезап."
	case "replicas":
		s.unit = "репл."
	case "pods":
		s.unit = "подов"
	case "targets":
		s.unit = "источн."
	}
	return s
}
func (s displayScale) format(value float64) string { n := value / s.divisor; return number(&n) }
func valueText(value *float64, unit string) string {
	if value == nil {
		return "нет данных"
	}
	s := scaleFor(unit, math.Abs(*value))
	return strings.TrimSpace(s.format(*value) + " " + s.unit)
}

func durationText(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%d с", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d мин", int(d.Minutes()))
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	if m == 0 {
		return fmt.Sprintf("%d ч", h)
	}
	return fmt.Sprintf("%d ч %d мин", h, m)
}
func right(value string, width int) string {
	return strings.Repeat(" ", max(0, width-tui.Width(value))) + value
}

func statistics(points []point, start, end float64) (low, mean, high float64, count int) {
	low, high = math.Inf(1), math.Inf(-1)
	for _, p := range points {
		if p.Value == nil || p.At < start || p.At > end {
			continue
		}
		count++
		// Incremental mean avoids overflowing a sum of otherwise valid samples.
		mean = mean*(float64(count-1)/float64(count)) + *p.Value/float64(count)
		low, high = math.Min(low, *p.Value), math.Max(high, *p.Value)
	}
	return
}

func changeText(s series, data *snapshot, unit string) string {
	if data == nil || current(s) == nil || len(s.Points) == 0 {
		return ""
	}
	first := s.Points[0]
	if first.Value == nil || first.At > data.Start+data.Step*1.5 {
		return ""
	}
	delta := *current(s) - *first.Value
	if math.Abs(delta) < 1e-9 {
		return "без изменений"
	}
	arrow := "↑"
	if delta < 0 {
		arrow = "↓"
	}
	if *first.Value != 0 {
		pct := math.Abs(delta / *first.Value * 100)
		if math.IsInf(pct, 0) || math.IsNaN(pct) {
			return ""
		}
		return arrow + " " + number(&pct) + "% к началу"
	}
	delta = math.Abs(delta)
	return arrow + " " + valueText(&delta, unit) + " к началу"
}
