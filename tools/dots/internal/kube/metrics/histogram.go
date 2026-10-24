package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Raw non-negative classic histogram rates; retain source labels so incompatible
// bucket layouts cannot silently be summed into a plausible-looking quantile.
type histogramSpec struct {
	Query        string   `json:"query"`
	Quantile     float64  `json:"quantile"`
	Scale        float64  `json:"scale"`
	CountScale   float64  `json:"count_scale"`
	GroupBy      []string `json:"group_by,omitempty"`
	ServiceQuery string   `json:"service_query,omitempty"`
	queryAt      func(string) string
}

func (h *histogramSpec) validate() error {
	if h == nil {
		return nil
	}
	if h.Query == "" || len(h.Query) > 8192 || len(h.ServiceQuery) > 8192 || !(h.Quantile > 0 && h.Quantile < 1) || !(h.Scale > 0 && h.Scale <= 1e9) || !(h.CountScale > 0 && h.CountScale <= 86400) || len(h.GroupBy) > 8 {
		return errors.New("histogram requires query, 0 < quantile < 1, positive scale/count_scale and bounded group_by")
	}
	seen := map[string]bool{}
	for _, label := range h.GroupBy {
		if !identifier.MatchString(label) || label == "le" || label == "__name__" || seen[label] {
			return errors.New("invalid histogram group_by")
		}
		seen[label] = true
	}
	return nil
}

type buckets map[float64]*float64
type histogramFrames struct {
	labels map[string]string
	frames map[float64]map[string]buckets // time → original source → cumulative buckets
}

func histogramPoints(ctx context.Context, rows []apiResult, history bool, h histogramSpec) (map[string]*histogramFrames, error) {
	result := map[string]*histogramFrames{}
	samples := 0
	seen := map[string]bool{}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		le, err := strconv.ParseFloat(row.Metric["le"], 64)
		if err != nil || math.IsNaN(le) || le < 0 {
			return nil, errors.New("histogram source requires nonnegative numeric le and +Inf buckets")
		}
		labels, source := map[string]string{}, map[string]string{}
		for k, v := range row.Metric {
			if k != "le" && k != "__name__" {
				source[k] = v
			}
		}
		for _, k := range h.GroupBy {
			if v := row.Metric[k]; v != "" {
				labels[k] = v
			}
		}
		key, sourceKey := labelKey(labels), labelKey(source)
		unique := sourceKey + "/" + strconv.FormatFloat(le, 'g', -1, 64)
		if seen[unique] {
			return nil, errors.New("duplicate histogram source bucket")
		}
		seen[unique] = true
		group := result[key]
		if group == nil {
			group = &histogramFrames{labels: labels, frames: map[float64]map[string]buckets{}}
			result[key] = group
		}
		values := row.Values
		if !history {
			values = [][]json.RawMessage{row.Value}
		}
		if len(values) > 1500 {
			return nil, errors.New("too many histogram points; increase step")
		}
		var last float64
		for i, raw := range values {
			samples++
			if samples > 120000 {
				return nil, errors.New("over 120000 raw histogram samples; narrow scope or increase step")
			}
			p, err := decodePoint(raw)
			if err != nil {
				return nil, err
			}
			if i > 0 && p.At <= last {
				return nil, errors.New("histogram samples are not ordered")
			}
			last = p.At
			frame := group.frames[p.At]
			if frame == nil {
				frame = map[string]buckets{}
				group.frames[p.At] = frame
			}
			if frame[sourceKey] == nil {
				frame[sourceKey] = buckets{}
			}
			frame[sourceKey][le] = p.Value
		}
	}
	if len(result) > 128 {
		return nil, errors.New("over 128 histogram groups; narrow scope")
	}
	return result, nil
}

func quantify(at float64, sources map[string]buckets, h histogramSpec) point {
	p := point{At: at, Reason: "invalid-histogram"}
	var bounds []float64
	var sums []float64
	for _, b := range sources {
		keys := make([]float64, 0, len(b))
		for le := range b {
			keys = append(keys, le)
		}
		sort.Float64s(keys)
		if len(keys) < 2 || !math.IsInf(keys[len(keys)-1], 1) {
			p.Reason = "incomplete-buckets"
			return p
		}
		if bounds == nil {
			bounds = keys
			sums = make([]float64, len(keys))
		}
		if len(keys) != len(bounds) {
			p.Reason = "incompatible-buckets"
			return p
		}
		previous := 0.0
		for i, le := range keys {
			if le != bounds[i] {
				p.Reason = "incompatible-buckets"
				return p
			}
			value := b[le]
			if value == nil || *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
				return p
			}
			n := *value
			if n < previous {
				if previous-n > 1e-12*math.Max(1, previous) {
					p.Reason = "nonmonotonic-buckets"
					return p
				}
				n = previous
			}
			sums[i] += n
			previous = n
			if math.IsInf(sums[i], 0) {
				return p
			}
		}
	}
	if len(bounds) < 2 {
		return p
	}
	total := sums[len(sums)-1]
	n := total * h.CountScale
	if math.IsInf(n, 0) {
		return p
	}
	p.Samples = &n
	if total == 0 {
		p.Reason = "no-observations"
		return p
	}
	rank := h.Quantile * total
	last := len(bounds) - 2
	if rank > sums[last] {
		lower := bounds[last] * h.Scale
		if math.IsInf(lower, 0) {
			return p
		}
		p.Lower = &lower
		p.Reason = "above-buckets"
		return p
	}
	lower, previous := 0.0, 0.0
	for i, upper := range bounds[:len(bounds)-1] {
		if sums[i] >= rank && sums[i] > previous {
			value := (lower + (upper-lower)*(rank-previous)/(sums[i]-previous)) * h.Scale
			lo, hi := lower*h.Scale, upper*h.Scale
			if math.IsInf(value, 0) || math.IsInf(hi, 0) {
				return p
			}
			p.Value, p.Lower, p.Upper = &value, &lo, &hi
			p.Reason = "estimate"
			return p
		}
		lower, previous = upper, sums[i]
	}
	return p
}

func (c *client) fetchHistogram(ctx context.Context, m metric, period, step time.Duration, now time.Time) (snapshot, error) {
	h := *m.Histogram
	end := float64(now.Unix())
	start := end - period.Seconds()
	data := snapshot{Metric: m.ID, Unit: m.Unit, Start: start, End: end, Step: step.Seconds(), Quantile: h.Quantile, Series: []series{}}
	query := h.Query
	if h.queryAt != nil {
		query = h.queryAt(strconv.FormatFloat(end, 'f', -1, 64))
	}
	currentRows, err := c.queryLimit(ctx, "/api/v1/query", url.Values{"query": {query}, "time": {strconv.FormatFloat(end, 'f', -1, 64)}}, "vector", 1024)
	if err != nil {
		return data, err
	}
	// Budget raw buckets independently of the <=20k final samples. Leave 2x
	// headroom for historical pods no longer present in the instant response.
	if len(currentRows) > 0 {
		capacity := max(2, 60000/len(currentRows))
		minimum := time.Duration(math.Ceil(period.Seconds()/float64(capacity-1))) * time.Second
		if minimum > time.Minute {
			minimum = ((minimum + time.Minute - 1) / time.Minute) * time.Minute
		}
		step = max(step, minimum)
		data.Step = step.Seconds()
	}
	args := url.Values{"query": {query}, "start": {strconv.FormatFloat(start, 'f', -1, 64)}, "end": {strconv.FormatFloat(end, 'f', -1, 64)}, "step": {strconv.FormatFloat(data.Step, 'f', -1, 64)}}
	historyRows, err := c.queryLimit(ctx, "/api/v1/query_range", args, "matrix", 1024)
	if err != nil {
		return data, err
	}
	history, err := histogramPoints(ctx, historyRows, true, h)
	if err != nil {
		return data, err
	}
	current, err := histogramPoints(ctx, currentRows, false, h)
	if err != nil {
		return data, err
	}
	keys := map[string]bool{}
	for k := range history {
		keys[k] = true
	}
	for k := range current {
		keys[k] = true
	}
	if len(keys) > 128 {
		return data, errors.New("over 128 histogram groups; narrow scope")
	}
	samples := 0
	for key := range keys {
		if err := ctx.Err(); err != nil {
			return data, err
		}
		s := series{Points: []point{}}
		if group := history[key]; group != nil {
			s.Labels = group.labels
			for at, frame := range group.frames {
				if err := ctx.Err(); err != nil {
					return data, err
				}
				s.Points = append(s.Points, quantify(at, frame, h))
			}
			sort.Slice(s.Points, func(i, j int) bool { return s.Points[i].At < s.Points[j].At })
		}
		if group := current[key]; group != nil {
			s.Labels = group.labels
			if len(group.frames) > 1 {
				return data, errors.New("inconsistent instant histogram timestamps")
			}
			for at, frame := range group.frames {
				p := quantify(at, frame, h)
				s.Current = &p
			}
		}
		samples += len(s.Points)
		if samples > 20000 {
			return data, errors.New("over 20000 histogram estimates; increase step")
		}
		data.Series = append(data.Series, s)
	}
	sort.Slice(data.Series, func(i, j int) bool { return labelKey(data.Series[i].Labels) < labelKey(data.Series[j].Labels) })
	data.FetchedAt = time.Now()
	return data, nil
}

func pointText(p *point, unit string) string {
	if p == nil {
		return "нет данных"
	}
	if p.Reason == "above-buckets" && p.Lower != nil {
		return "≥ " + valueText(p.Lower, unit)
	}
	text := valueText(p.Value, unit)
	if p.Reason == "estimate" {
		text = "≈ " + text
	}
	return text
}

func quantileNote(p *point, unit string) string {
	if p == nil || p.Reason == "" {
		return ""
	}
	switch p.Reason {
	case "above-buckets":
		return "Выше границы гистограммы: точный перцентиль неизвестен. Нужны более широкие buckets в источнике."
	case "no-observations":
		return "За окно нет наблюдений: перцентиль не определён, это не нулевая задержка."
	case "incompatible-buckets":
		return "У источников разные границы buckets: объединять такие гистограммы нельзя."
	case "nonmonotonic-buckets", "incomplete-buckets", "invalid-histogram":
		return "Некорректная или неполная гистограмма: оценка скрыта, проверьте источник."
	case "estimate":
		if p.Lower != nil && p.Upper != nil {
			return "Оценка внутри bucket: " + valueText(p.Lower, unit) + " … " + valueText(p.Upper, unit) + "; точность ограничена границами."
		}
	}
	return ""
}

func sampleNote(p *point, q float64) string {
	if p == nil || p.Samples == nil {
		return ""
	}
	text := fmt.Sprintf("Наблюдений за окно: ≈ %s", number(p.Samples))
	if q > 0 && *p.Samples*(1-q) < 10 {
		text += " · мало для устойчивого хвостового перцентиля"
	}
	return text
}
