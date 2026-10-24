package metrics

import (
	"errors"
	"regexp"
	"strconv"
)

func validateRouteFilter(filter string) error {
	if len(filter) > 512 {
		return errors.New("route filter is limited to 512 bytes")
	}
	if _, err := regexp.Compile(filter); err != nil {
		return errors.New("invalid route filter regular expression (details hidden)")
	}
	return nil
}

// Optional route instrumentation. Select an explicit top ten at the END of the
// interval and keep that same cohort for its entire history. A moving topk per
// step would invent gaps as routes leave the ranking and can exceed 128 series.
func routeMetrics(p profile) []metric {
	if p.RouteLabel == "" {
		return nil
	}
	labels := "namespace=" + strconv.Quote(p.Namespace)
	if p.Service != "" {
		labels += ",service=" + strconv.Quote(p.Service)
	}
	labels += "," + p.RouteLabel + `!=""`
	if p.RouteFilter != "" {
		labels += "," + p.RouteLabel + "=~" + strconv.Quote(p.RouteFilter)
	}
	group := "service," + p.RouteLabel + ",method"
	durationMetric := p.RouteDurationMetric
	if durationMetric == "" {
		durationMetric = p.DurationMetric
	}
	rate := func(extra, at string) string {
		return "sum by (" + group + ") (rate(" + p.RequestMetric + "{" + labels + extra + "}[1m]" + at + "))"
	}
	latency := func(q, at string) string {
		return "1000 * histogram_quantile(" + q + ", sum by (le," + group + ") (rate(" + durationMetric + "_bucket{" + labels + "}[5m]" + at + ")))"
	}
	errorRate := func(at string) string { return rate(","+p.StatusLabel+`=~"5([0-9]{2}|xx)"`, at) }
	errorPercent := func(at string) string { return "100 * " + errorRate(at) + " / on(" + group + ") " + rate("", at) }
	scope := "HTTP: все сервисы · топ-10 сейчас"
	if p.Service != "" {
		scope = "HTTP: " + p.Service + " · топ-10 сейчас"
	}
	if p.RouteFilter != "" {
		scope += " · фильтр маршрута: " + p.RouteFilter
	}
	makeMetric := func(id, title, unit string, query func(string) string, description string) metric {
		rank := func(end string) string {
			value := query(" @ " + end)
			if id == "routep95" || id == "routep99" {
				// Identity fallback for idle routes, NOT a zero latency sample.
				// Their raw buckets still produce null/no-observations currently.
				value = "(" + value + " >= 0) or on(" + group + ") (0 * " + rate("", " @ "+end) + ")"
			}
			return value
		}
		stable := func(end string) string {
			return "(" + query("") + ") and on(" + group + ") topk(10, " + rank(end) + ")"
		}
		m := metric{ID: id, Title: title, Unit: unit, Query: stable("end()"), queryAt: stable, Description: description + " Топ-10 на конец периода; история этих же ручек, не меняющийся топ каждой точки. Маршруты и методы разных сервисов не смешиваются. --route REGEX сужает выборку.", Scope: scope}
		if id == "routep95" || id == "routep99" {
			q := 0.95
			if id == "routep99" {
				q = 0.99
			}
			raw := func(end string) string {
				return "rate(" + durationMetric + "_bucket{" + labels + "}[5m]) and on(" + group + ") topk(10, " + rank(end) + ")"
			}
			m.Histogram = &histogramSpec{Query: raw("end()"), queryAt: raw, Quantile: q, Scale: 1000, CountScale: 300, GroupBy: []string{"service", p.RouteLabel, "method"}}
		}
		return m
	}
	return []metric{
		makeMetric("routerps", "Топ-10 ручек · RPS", "req/s", func(at string) string { return rate("", at) }, "Запросы в секунду за минуту по маршрутам и методам."),
		makeMetric("routep95", "Топ-10 ручек · p95", "ms", func(at string) string { return latency("0.95", at) }, "p95 за 5 минут по маршрутам. Объединяем buckets, а не усредняем перцентили подов. При малом трафике оценка нестабильна."),
		makeMetric("routep99", "Топ-10 ручек · p99", "ms", func(at string) string { return latency("0.99", at) }, "p99 за 5 минут по маршрутам. 1% запросов медленнее; смотрите вместе с нагрузкой."),
		makeMetric("routeerrors", "Топ-10 ручек · 5xx %", "%", errorPercent, "Доля 5xx за минуту. При малом трафике процент нестабилен; сравните с ошибками в секунду. Отсутствующий ряд ошибок не превращается в ноль."),
		makeMetric("routeerrorsrate", "Топ-10 ручек · 5xx/с", "req/s", errorRate, "Число ответов 5xx в секунду за минуту. Поддерживаются коды 500 и классы 5xx."),
	}
}
