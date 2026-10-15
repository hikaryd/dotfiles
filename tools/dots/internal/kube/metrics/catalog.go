package metrics

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

// The catalog is intentionally exporter-specific, not a claim that every
// cluster has these series. Missing inputs remain unknown, never `or vector(0)`.
func builtins(p profile) []metric {
	labels := "namespace=" + strconv.Quote(p.Namespace)
	httpLabels := labels
	if p.Service != "" {
		httpLabels += ",service=" + strconv.Quote(p.Service)
	}
	requests := func(extra string) string {
		if extra != "" {
			extra = "," + extra
		}
		return p.RequestMetric + "{" + httpLabels + extra + "}"
	}
	rate := func(extra string) string { return "sum(rate(" + requests(extra) + "[1m]))" }
	container := labels + `,container!="",container!="POD"`
	cpu := "sum(rate(container_cpu_usage_seconds_total{" + container + "}[5m]))"
	memory := "sum(container_memory_working_set_bytes{" + container + "})"
	latency := func(q, group string) string {
		return "1000 * histogram_quantile(" + q + ", sum by (le" + group + ") (rate(" + p.DurationMetric + "_bucket{" + httpLabels + "}[5m])))"
	}
	// Ratios join by pod/container before aggregating. This excludes containers
	// without a declared request/limit from BOTH numerator and denominator.
	allocation := func(resource, kind, usage string) string {
		declared := "(kube_pod_container_resource_" + kind + "{" + labels + ",resource=" + strconv.Quote(resource) + "} > 0)"
		used := "sum by (namespace,pod,container) (" + usage + ")"
		return "100 * sum(" + used + " and on(namespace,pod,container) " + declared + ") / sum(" + declared + " and on(namespace,pod,container) " + used + ")"
	}
	m := func(id, title, unit, query, description string) metric {
		m := metric{ID: id, Title: title, Unit: unit, Query: query, Description: description}
		if id == "p95" || id == "p99" || id == "latencyservices" || id == "latencyservices99" {
			q := 0.95
			if id == "p99" || id == "latencyservices99" {
				q = 0.99
			}
			m.Histogram = &histogramSpec{Query: "rate(" + p.DurationMetric + "_bucket{" + httpLabels + "}[5m])", Quantile: q, Scale: 1000, CountScale: 300}
			if id == "latencyservices" || id == "latencyservices99" {
				m.Histogram.GroupBy = []string{"service"}
			}
		}
		return m
	}
	return []metric{
		m("rps", "Запросы в секунду (RPS)", "req/s", rate(""), "Среднее число HTTP-запросов в секунду за предыдущую минуту. Например: 600 запросов/мин = 10 запросов/с. При редком сборе нужно расширить окно расчёта."),
		m("rpm", "Запросы за минуту", "req/min", "sum(increase("+requests("")+"[1m]))", "Оценка числа запросов за предыдущую минуту. Не RPS. Возможны дробные значения: счётчик экстраполируется на границы минуты."),
		m("services", "Нагрузка по сервисам", "req/s", "sum by (service) (rate("+requests("")+"[1m]))", "HTTP-запросы в секунду за последнюю минуту. Сервисы отсортированы по текущей нагрузке. Учитывается фильтр сервиса из профиля."),
		m("statuses", "HTTP-ответы по классам", "req/s", "sum by (class) (label_replace(rate("+requests(p.StatusLabel+`=~"[1-5]([0-9]{2}|xx)"`)+`[1m]), "class", "${1}xx", "`+p.StatusLabel+`", "([1-5])([0-9]{2}|xx)"))`, "2xx — успешно; 3xx — перенаправления; 4xx — ошибки запроса; 5xx — ошибки сервера. Поддерживаются коды 200 и уже сгруппированные классы 2xx."),
		m("errors5xx", "Ошибки сервера · 5xx", "%", "100 * "+rate(p.StatusLabel+`=~"5([0-9]{2}|xx)"`)+" / "+rate(""), "Доля HTTP-запросов с ответом 5xx за последнюю минуту. Рядом — число ошибок в секунду: при малом трафике один процент мало что объясняет."),
		m("errors5xxrate", "Ошибки 5xx в секунду", "req/s", rate(p.StatusLabel+`=~"5([0-9]{2}|xx)"`), "Среднее число ответов 5xx в секунду за предыдущую минуту. Сравнивайте с общим RPS, долей ошибок и временем ответа."),
		m("errors4xx", "Ошибки запроса · 4xx", "%", "100 * "+rate(p.StatusLabel+`=~"4([0-9]{2}|xx)"`)+" / "+rate(""), "Доля ответов 4xx за минуту. Включает неверные параметры и отказ в доступе — не каждый 4xx означает неисправность сервиса."),
		m("errors4xxrate", "Ошибки 4xx в секунду", "req/s", rate(p.StatusLabel+`=~"4([0-9]{2}|xx)"`), "Число ответов 4xx в секунду за предыдущую минуту. Сравнивайте с трафиком и классами HTTP-ответов."),
		m("errorservices", "Ошибки 5xx по сервисам", "req/s", "sum by (service) (rate("+requests(p.StatusLabel+`=~"5([0-9]{2}|xx)"`)+"[1m]))", "Сервисы отсортированы по числу ошибок 5xx в секунду, не по проценту. Отсутствующий ряд — не доказанный ноль."),
		m("p95", "Время ответа · p95", "ms", latency("0.95", ""), "95% запросов быстрее этого значения, 5% — медленнее. Расчёт по общей гистограмме за 5 минут, не среднее перцентилей сервисов."),
		m("p99", "Время ответа · p99", "ms", latency("0.99", ""), "99% запросов быстрее этого значения, 1% — медленнее. Расчёт за 5 минут. При малом трафике редкие медленные запросы заметно меняют p99."),
		m("latencyservices", "Время ответа по сервисам", "ms", latency("0.95", ",service"), "p95 за 5 минут: 95% запросов сервиса быстрее этого значения. Сервисы отсортированы от медленных к быстрым. При малом трафике перцентиль нестабилен."),
		m("latencyservices99", "Время по сервисам · p99", "ms", latency("0.99", ",service"), "p99 за 5 минут по сервисам. Крупные или неполные buckets ограничивают точность; это не p95."),
		m("cpu", "Использование CPU", "cores", cpu, "Средняя загрузка за 5 минут во всём namespace. 1 ядро = один полностью занятый vCPU. Фильтр HTTP-сервиса к ресурсам не применяется."),
		m("cpurequests", "CPU / резерв", "%", allocation("cpu", "requests", "rate(container_cpu_usage_seconds_total{"+container+"}[5m])"), "Потребление CPU относительно requests (резерва). Только контейнеры с заданным резервом и измерением. Выше 100% — выше резерва, не обязательно перегрузка."),
		m("cpulimits", "CPU / лимит", "%", allocation("cpu", "limits", "rate(container_cpu_usage_seconds_total{"+container+"}[5m])"), "Потребление CPU относительно limits. Только контейнеры с заданным лимитом и измерением. Общий запас может скрыть перегрузку одного контейнера."),
		m("throttling", "Ограничение CPU", "%", "100 * sum(rate(container_cpu_cfs_throttled_periods_total{"+container+"}[5m])) / sum(rate(container_cpu_cfs_periods_total{"+container+"}[5m]))", "Доля CFS-периодов, в которых CPU ограничивался, за 5 минут. Это не доля потерянного процессорного времени. Смотрите вместе со временем ответа."),
		m("cpupods", "CPU по подам", "cores", "sum by (pod) (rate(container_cpu_usage_seconds_total{"+container+"}[5m]))", "Поды отсортированы по использованию CPU за 5 минут. Все поды namespace, независимо от HTTP-фильтра."),
		m("memory", "Использование памяти", "MiB", memory+" / 1024 / 1024", "Рабочий набор памяти контейнеров во всём namespace. Не равен размеру heap приложения. 1 ГиБ = 1024 МиБ."),
		m("memoryrequests", "Память / резерв", "%", allocation("memory", "requests", "container_memory_working_set_bytes{"+container+"}"), "Рабочий набор относительно requests. Только контейнеры с резервом и измерением. Резерв памяти — не порог OOM."),
		m("memorylimits", "Память / лимит", "%", allocation("memory", "limits", "container_memory_working_set_bytes{"+container+"}"), "Рабочий набор относительно limits. Только контейнеры с лимитом и измерением. Общий процент может скрывать один контейнер у лимита."),
		m("memorypods", "Память по подам", "MiB", "sum by (pod) (container_memory_working_set_bytes{"+container+"}) / 1024 / 1024", "Поды отсортированы по рабочему набору памяти. Все поды namespace, независимо от HTTP-фильтра."),
		m("restarts", "Перезапуски за 15 мин", "restarts", "sum(increase(kube_pod_container_status_restarts_total{"+labels+"}[15m]))", "Рост счётчика перезапусков контейнеров за 15 минут. Дробные значения — оценка по счётчику. Причину, в том числе OOM, смотрите в статусе и логах."),
		m("restartpods", "Перезапуски по подам", "restarts", "sum by (pod) (increase(kube_pod_container_status_restarts_total{"+labels+"}[15m]))", "Перезапуски контейнеров за 15 минут, по подам. Из-за экстраполяции возможны дробные значения. Нет истории — не значит ноль перезапусков."),
		m("unavailable", "Недоступные реплики", "replicas", "clamp_min(sum(kube_deployment_spec_replicas{"+labels+"}) - sum(kube_deployment_status_replicas_available{"+labels+"}), 0)", "Сколько реплик Deployment не хватает до желаемого количества. Во время выкладки это может быть временно. StatefulSet/DaemonSet не входят."),
		m("workloads", "Недоступные Deployment", "replicas", "clamp_min(kube_deployment_spec_replicas{"+labels+"} - on(namespace,deployment) kube_deployment_status_replicas_available{"+labels+"}, 0)", "Deployment отсортированы по числу недоступных реплик. Перед выводом об аварии проверьте, не идёт ли выкладка."),
		m("pending", "Поды в ожидании", "pods", "sum(kube_pod_status_phase{"+labels+`,phase="Pending"`+"})", "Поды в фазе Pending. Временное ожидание при выкладке нормально; длительное — повод проверить события и планировщик."),
		m("notready", "Поды не готовы", "pods", "sum((kube_pod_status_ready{"+labels+`,condition="true"`+"} == bool 0) * on(namespace,pod) (kube_pod_status_phase{"+labels+`,phase="Running"`+"} == 1))", "Поды запущены, но не готовы принимать трафик (Running, Ready=false). Завершённые задания и Pending не учитываются."),
		m("scrapes", "Ошибки сбора метрик", "targets", "sum(1 - up{"+labels+"})", "Источники с неудачным сбором. Наблюдения могут быть неполными; это не обязательно неисправность самого приложения. Требуется метка namespace на up."),
	}
}

var companions = map[string]string{"errors5xx": "errors5xxrate", "errors4xx": "errors4xxrate"}
var breakdowns = map[string]string{
	"rps": "services", "rpm": "services", "errors5xx": "errorservices", "errors5xxrate": "errorservices",
	"p95": "latencyservices", "p99": "latencyservices99", "cpu": "cpupods", "cpurequests": "cpupods", "cpulimits": "cpupods",
	"memory": "memorypods", "memoryrequests": "memorypods", "memorylimits": "memorypods", "unavailable": "workloads", "restarts": "restartpods",
}

func findMetric(items []metric, id string) (int, error) {
	for i, m := range items {
		if m.ID == id {
			return i, nil
		}
	}
	return 0, errors.New("unknown metric ID; use --list")
}

func seriesLabel(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		if key != "__name__" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	if len(parts) == 0 {
		return "Всего"
	}
	return strings.Join(parts, ", ")
}
