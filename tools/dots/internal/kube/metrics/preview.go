package metrics

import (
	"fmt"
	"strings"
	"time"
)

// Synthetic samples intentionally include zeros, exporter gaps, an unavailable
// workload and low-volume 5xx. Never use real coordinates in previews/tests.
func preview(scenario string, period, step time.Duration) view {
	p := profile{Name: "demo · SYNTHETIC", Namespace: "example", RequestMetric: "http_requests_total", DurationMetric: "http_request_duration_seconds", StatusLabel: "status", RouteLabel: "handler"}
	p.Queries = previewQueries()
	if scenario == "missing" {
		p.RouteLabel = ""
		var configured []metric
		for _, m := range p.Queries {
			if m.Section != "runtime" && m.Section != "outbound" && m.Section != "postgres" {
				configured = append(configured, m)
			}
		}
		p.Queries = configured
		p.SectionNotes = map[string]string{"postgres": "Синтетический пример: метрики PostgreSQL не подключены.", "runtime": "Синтетический пример: collector runtime приложения не настроен.", "outbound": "Синтетический пример: метрики исходящих вызовов ещё не собираются."}
	}
	v := newView(p, period, step)
	v.services, v.servicesLoaded = []string{"api", "gateway", "scheduler", "worker"}, true
	now := time.Now()
	end := float64(now.Unix())
	values := map[string]float64{"rps": 420, "rpm": 25200, "errors5xx": 0.12, "errors5xxrate": 0.504, "errors4xx": 1.7, "errors4xxrate": 7.14, "p95": 145, "p99": 310, "cpu": 3.4, "cpurequests": 68, "cpulimits": 34, "throttling": 0.8, "memory": 1820, "memoryrequests": 88, "memorylimits": 44, "unavailable": 0, "notready": 0, "pending": 0, "restarts": 0, "scrapes": 0}
	for id, value := range map[string]float64{"demo-db-pool": 8, "demo-db-wait": 4, "demo-queue-depth": 12, "demo-queue-age": 8, "demo-ingress-p95": 160, "demo-probe": 100} {
		values[id] = value
	}
	if scenario == "degraded" {
		values["demo-db-wait"], values["demo-queue-age"], values["demo-outbound-errors"], values["demo-probe"] = 900, 480, 12, 0
		values["errors5xx"] = 4.8
		values["errors5xxrate"] = 20.16
		values["p95"] = 940
		values["unavailable"] = 2
		values["notready"] = 3
		values["restarts"] = 7
	}
	for _, m := range v.items {
		data := snapshot{Metric: m.ID, Unit: m.Unit, FetchedAt: now, Start: end - period.Seconds(), End: end, Step: v.step.Seconds(), Series: []series{}}
		if scenario == "missing" && (m.ID == "p95" || m.ID == "p99" || m.ID == "unavailable" || m.ID == "notready") {
			v.readings[m.ID] = reading{Data: &data}
			continue
		}
		count := 1
		switch m.ID {
		case "services", "errorservices", "latencyservices", "latencyservices99", "cpupods", "memorypods", "restartpods", "workloads", "statuses", "routerps", "routep95", "routep99", "routeerrors", "routeerrorsrate", "demo-ingress-p95", "demo-probe", "demo-redis-clients", "demo-pg-connections":
			count = 4
		}
		for j := 0; j < count; j++ {
			labels := map[string]string{}
			value, ok := values[m.ID]
			if !ok {
				value = 12 + float64(j*5)
			}
			if count > 1 {
				labels["service"] = []string{"api", "worker", "gateway", "scheduler"}[j]
				if strings.HasPrefix(m.ID, "route") {
					labels["handler"], labels["method"] = []string{"/items", "/items/{id}", "/search", "/health"}[j], []string{"GET", "POST", "PATCH", "DELETE"}[j]
				}
			}
			switch m.ID {
			case "demo-redis-clients":
				labels = map[string]string{"instance": fmt.Sprintf("redis-%d", j)}
			case "demo-pg-connections":
				labels = map[string]string{"instance": fmt.Sprintf("postgres-%d", j)}
			case "demo-probe":
				labels = map[string]string{"target": fmt.Sprintf("https://example-%d.invalid", j)}
			}
			s := series{Labels: labels, Points: []point{}}
			for at := data.Start; at <= end; at += v.step.Seconds() {
				n := value
				if value > 0 && m.ID != "demo-probe" {
					n *= 0.8 + float64(int(at/60)%13)/30
				}
				s.Points = append(s.Points, point{At: at, Value: &n})
			}
			if len(s.Points) > 0 && s.Points[len(s.Points)-1].At == end {
				s.Points[len(s.Points)-1].Value = &value
			}
			s.Current = &point{At: end, Value: &value}
			if m.Histogram != nil {
				data.Quantile = m.Histogram.Quantile
				decorate := func(p *point) {
					n, lo, hi := 5000.0, *p.Value*.8, *p.Value*1.2
					p.Reason = "estimate"
					p.Lower = &lo
					p.Upper = &hi
					p.Samples = &n
					if scenario == "censored" {
						lo, n = 1000, 3
						p.Value = nil
						p.Lower = &lo
						p.Upper = nil
						p.Samples = &n
						p.Reason = "above-buckets"
					}
				}
				decorate(s.Current)
				for i := range s.Points {
					decorate(&s.Points[i])
				}
			}
			data.Series = append(data.Series, s)
		}
		v.readings[m.ID] = reading{Data: &data}
	}
	if scenario == "error" {
		for id, r := range v.readings {
			r.Error = "metrics API returned HTTP 503 (body hidden)"
			r.Data.FetchedAt = now.Add(-90 * time.Second)
			v.readings[id] = r
		}
	}
	return v
}

// Illustrative fixtures, not a claim that any particular exporter is installed.
func previewQueries() []metric {
	return []metric{
		{ID: "demo-pg-connections", Title: "Подключения PostgreSQL", Unit: "connections", Section: "postgres", Query: "example_pg_connections", Description: "Синтетика: подключения отдельного экземпляра PostgreSQL."},
		{ID: "demo-redis-clients", Title: "Клиенты Redis", Unit: "clients", Section: "redis", Query: "example_redis_clients", Description: "Синтетика: клиенты каждого экземпляра Redis, без суммирования реплик."},
		{ID: "demo-db-pool", Title: "Занятые соединения", Unit: "connections", Section: "db", Scope: "пул example-api · синтетика", Query: "example_pool_active", Description: "Соединения, занятые приложением. Сравнивайте с максимальным размером пула и временем ожидания."},
		{ID: "demo-db-wait", Title: "Ожидание пула · p95", Unit: "ms", Section: "db", Query: "example_pool_wait_p95", Description: "Время ожидания свободного соединения. Это не длительность SQL-запроса."},
		{ID: "demo-queue-depth", Title: "Ожидающие задачи", Unit: "jobs", Section: "queues", Query: "example_queue_depth", Description: "Размер очереди. Смотрите вместе с возрастом задачи и скоростью обработки."},
		{ID: "demo-queue-age", Title: "Возраст старой задачи", Unit: "s", Section: "queues", Query: "example_queue_oldest_age_seconds", Description: "Возраст самой старой ожидающей задачи. Даже небольшая очередь может стоять."},
		{ID: "demo-ingress-p95", Title: "Время шлюза · p95", Unit: "ms", Section: "ingress", Query: "example_ingress_p95", Description: "Полное время на входном шлюзе, включая upstream."},
		{ID: "demo-probe", Title: "Успех внешней проверки", Unit: "%", Section: "ingress", Query: "100 * example_probe_success", Description: "100% — проверка успешна, 0% — неуспешна. Это последняя проба, не доля доступности за период."},
	}
}
