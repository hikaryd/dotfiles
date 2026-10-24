package metrics

import (
	"math"
	"strings"
	"time"

	"dots/native/internal/tui"
)

const (
	upStroke uint8 = 1 << iota
	rightStroke
	downStroke
	leftStroke
	pointStroke
)

// Connect actual adjacent samples, not adjacent terminal buckets. A missing or
// non-finite sample breaks the line. Repeated x columns still keep brief peaks.
func lineGrid(points []point, start, end, step float64, columns, rows int, low, high float64) [][]uint8 {
	grid := make([][]uint8, rows)
	for y := range grid {
		grid[y] = make([]uint8, columns)
	}
	if end <= start || high <= low {
		return grid
	}
	position := func(p point) (int, int) {
		x := min(columns-1, max(0, int(math.Round((p.At-start)/(end-start)*float64(columns-1)))))
		y := min(rows-1, max(0, int(math.Round((high/2-*p.Value/2)/(high/2-low/2)*float64(rows-1)))))
		return x, y
	}
	join := func(x, y, nx, ny int) {
		if x != nx {
			grid[y][x] |= rightStroke
			grid[ny][nx] |= leftStroke
		} else if y < ny {
			grid[y][x] |= downStroke
			grid[ny][nx] |= upStroke
		} else if y > ny {
			grid[y][x] |= upStroke
			grid[ny][nx] |= downStroke
		}
	}
	var previous *point
	for _, p := range points {
		if p.Value == nil || p.At < start || p.At > end {
			previous = nil
			continue
		}
		x, y := position(p)
		grid[y][x] |= pointStroke
		if previous != nil && step > 0 && p.At-previous.At > 0 && p.At-previous.At <= step*1.5 {
			px, py := position(*previous)
			initialY := py
			// Orthogonal line with rounded corners: continuous at any TTY width.
			for nextX := px; nextX <= x; nextX++ {
				nextY := y
				if x > px {
					nextY = int(math.Round(float64(initialY) + float64(y-initialY)*float64(nextX-px)/float64(x-px)))
				}
				if nextX > px {
					join(nextX-1, py, nextX, py)
				}
				for py != nextY {
					ny := py + 1
					if nextY < py {
						ny = py - 1
					}
					join(nextX, py, nextX, ny)
					py = ny
				}
			}
		}
		copy := p
		previous = &copy
	}
	return grid
}

func stroke(mask uint8) rune {
	switch mask & 15 {
	case 0:
		if mask&pointStroke != 0 {
			return '◆'
		}
		return ' '
	case rightStroke, leftStroke, rightStroke | leftStroke:
		return '─'
	case upStroke, downStroke, upStroke | downStroke:
		return '│'
	case rightStroke | downStroke:
		return '╭'
	case leftStroke | downStroke:
		return '╮'
	case rightStroke | upStroke:
		return '╰'
	case leftStroke | upStroke:
		return '╯'
	case upStroke | downStroke | rightStroke:
		return '├'
	case upStroke | downStroke | leftStroke:
		return '┤'
	case leftStroke | rightStroke | upStroke:
		return '┴'
	case leftStroke | rightStroke | downStroke:
		return '┬'
	default:
		return '┼'
	}
}

func axisBounds(low, high float64) (float64, float64) {
	low, high = math.Min(0, low), math.Max(0, high)
	if low == high {
		return 0, 1
	}
	// Nice quarter-scale labels, e.g. 0 / 2.5 / 5 / 7.5 / 10 instead of 9.12 / 0.
	span := high/4 - low/4
	power := math.Pow(10, math.Floor(math.Log10(span)))
	if power == 0 || math.IsInf(power, 0) {
		return low, high
	}
	tick := power * 10
	for _, candidate := range []float64{1, 2, 2.5, 5, 10} {
		if candidate*power >= span {
			tick = candidate * power
			break
		}
	}
	bottom, top := math.Floor(low/tick)*tick, math.Ceil(high/tick)*tick
	if math.IsInf(bottom, 0) || math.IsInf(top, 0) {
		return low, high
	}
	return bottom, top
}

func historyChart(s series, data *snapshot, unit string, width, rows int, color bool) []string {
	low, _, high, count := statistics(s.Points, data.Start, data.End)
	if count == 0 {
		for _, p := range s.Points {
			if p.Reason == "above-buckets" {
				return []string{"Есть только нижние границы. Точный график по этим buckets построить нельзя."}
			}
		}
		return []string{"В выбранном периоде нет измерений для графика."}
	}
	scale := scaleFor(unit, math.Max(math.Abs(low), math.Abs(high)))
	low, high = axisBounds(low/scale.divisor, high/scale.divisor)
	points := make([]point, len(s.Points))
	for i, p := range s.Points {
		points[i] = p
		if p.Value != nil {
			n := *p.Value / scale.divisor
			points[i].Value = &n
		}
	}
	labels := map[int]string{}
	for i := range min(5, rows) {
		y := int(math.Round(float64(i) * float64(rows-1) / float64(min(5, rows)-1)))
		fraction := float64(i) / float64(min(5, rows)-1)
		value := high*(1-fraction) + low*fraction
		labels[y] = number(&value)
	}
	axisWidth := max(6, tui.Width(scale.unit))
	for _, label := range labels {
		axisWidth = max(axisWidth, tui.Width(label))
	}
	axisWidth = min(axisWidth, max(6, width/3))
	columns := max(2, width-axisWidth-2)
	grid := lineGrid(points, data.Start, data.End, data.Step, columns, rows, low, high)
	out := []string{right(scale.unit, axisWidth) + " │"}
	for y, cells := range grid {
		axis := "│"
		label := labels[y]
		if label != "" {
			axis = "┤"
		}
		line := tui.Paint(right(tui.Clip(label, axisWidth), axisWidth)+" "+axis, tui.Dim, color)
		// Paint runs, not individual cells, to keep redraw output bounded.
		var run strings.Builder
		previousTone := ""
		flush := func() {
			if run.Len() > 0 {
				line += tui.Paint(run.String(), previousTone, color)
				run.Reset()
			}
		}
		for _, cell := range cells {
			r, tone := stroke(cell), tui.Accent
			if r == ' ' {
				tone = tui.Border
				if label != "" {
					r = '┄'
				}
			}
			if tone != previousTone {
				flush()
				previousTone = tone
			}
			run.WriteRune(r)
		}
		flush()
		out = append(out, line)
	}
	baseline, times := timeAxis(data.Start, data.End, columns)
	out = append(out, tui.Paint(strings.Repeat(" ", axisWidth+1)+"└"+baseline, tui.Dim, color))
	out = append(out, strings.Repeat(" ", axisWidth+2)+times)
	return out
}

func timeAxis(start, end float64, columns int) (string, string) {
	first, last := time.Unix(int64(start), 0).Local(), time.Unix(int64(end), 0).Local()
	format := "15:04"
	if first.Format("2006-01-02") != last.Format("2006-01-02") || end-start >= 12*3600 {
		format = "02.01 15:04"
	}
	labelWidth := len(first.Format(format))
	count := min(7, max(1, columns/(labelWidth+5)))
	if columns >= 2*labelWidth+1 {
		count = max(2, count)
	}
	baseline := []rune(strings.Repeat("─", columns))
	text := []rune(strings.Repeat(" ", columns))
	for i := range count {
		fraction := 0.0
		if count > 1 {
			fraction = float64(i) / float64(count-1)
		}
		x := int(math.Round(fraction * float64(columns-1)))
		baseline[x] = '┬'
		label := []rune(time.Unix(int64(start+(end-start)*fraction), 0).Local().Format(format))
		if len(label) > columns {
			continue
		}
		position := min(columns-len(label), max(0, x-len(label)/2))
		copy(text[position:], label)
	}
	return string(baseline), string(text)
}

// Compact overview only. Detail uses the connected line above.
func spark(points []point, start, end float64, columns int) string {
	buckets := plotBuckets(points, start, end, columns)
	low, high := math.Inf(1), math.Inf(-1)
	for _, b := range buckets {
		if b.Present {
			low = math.Min(low, b.Low)
			high = math.Max(high, b.High)
		}
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	out := make([]rune, columns)
	for i, b := range buckets {
		out[i] = '·'
		if b.Present {
			level := 0
			if high > low {
				level = min(7, max(0, int(math.Round((b.High-low)/(high-low)*7))))
			}
			out[i] = bars[level]
		}
	}
	return string(out)
}

type plotBucket struct {
	Low, High float64
	Present   bool
}

func plotBuckets(points []point, start, end float64, columns int) []plotBucket {
	buckets := make([]plotBucket, columns)
	for _, p := range points {
		if p.Value == nil || p.At < start || p.At > end || end <= start {
			continue
		}
		x := min(columns-1, max(0, int((p.At-start)/(end-start)*float64(columns-1))))
		b := &buckets[x]
		if !b.Present {
			b.Low = *p.Value
			b.High = *p.Value
			b.Present = true
		} else {
			b.Low = math.Min(b.Low, *p.Value)
			b.High = math.Max(b.High, *p.Value)
		}
	}
	return buckets
}
