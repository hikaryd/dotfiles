package metrics

import (
	"errors"
	"fmt"
	"slices"

	"dots/native/internal/tui"
)

// One registry drives navigation, CLI targets, validation and setup guidance.
// Legacy section IDs stay valid; navigation omits unconfigured optional groups.
var sectionShortcuts = []string{"overview", "http", "resources", "health", "routes"}

type section struct {
	ID, Title, Setup string
	Metrics          []string
}

var sections = []section{
	{ID: "overview", Title: "Обзор", Metrics: []string{"rps", "errors5xx", "p95", "cpu", "memory", "unavailable", "notready", "restarts"}},
	{ID: "http", Title: "HTTP", Metrics: []string{"rps", "rpm", "errors5xx", "errors4xx", "p95", "p99", "statuses", "services"}},
	{ID: "resources", Title: "Ресурсы", Metrics: []string{"cpu", "cpurequests", "cpulimits", "throttling", "memory", "memoryrequests", "memorylimits"}},
	{ID: "health", Title: "Состояние", Metrics: []string{"unavailable", "notready", "pending", "restarts", "scrapes"}},
	{ID: "custom", Title: "Свои", Setup: "Добавьте queries в приватный metrics.json. Без section запросы попадают сюда."},
	{ID: "routes", Title: "HTTP-ручки", Metrics: []string{"routerps", "routep95", "routep99", "routeerrors", "routeerrorsrate"}, Setup: "Нужны HTTP-счётчик и гистограмма с label шаблона маршрута. Задайте route_label (например, handler) в приватном профиле. Не используйте URL с идентификаторами пользователей."},
	{ID: "postgres", Title: "PostgreSQL", Setup: "Подключите postgres_exporter или метрики оператора, затем добавьте queries с section: postgres."},
	{ID: "redis", Title: "Redis", Setup: "Подключите redis_exporter и добавьте queries с section: redis."},
	{ID: "db", Title: "Пулы и другие БД", Setup: "Нужны exporter БД и/или метрики пула приложения: соединения, ожидание, ошибки, длительность операций. Добавьте queries с section: db; фильтр источника задаётся явно."},
	{ID: "queues", Title: "Очереди", Setup: "Нужны метрики брокера и воркеров: ожидающие задачи, возраст старой задачи, скорость обработки, ошибки. Добавьте queries с section: queues. Число поставленных задач не равно длине очереди."},
	{ID: "outbound", Title: "Исходящие вызовы", Setup: "Нужны метрики HTTP/RPC-клиентов приложения: задержки, ошибки, таймауты, повторы по зависимости. Добавьте queries с section: outbound. Входящий HTTP и запросы Kubernetes не заменяют эти данные."},
	{ID: "ingress", Title: "Вход и доступность", Setup: "Нужны метрики ingress и внешние HTTP-пробы. Добавьте queries с section: ingress. Проверяйте привязку route/service/target к приложению: namespace контроллера может отличаться."},
	{ID: "runtime", Title: "Runtime", Setup: "Нужны метрики runtime самого приложения: heap, GC, потоки, event loop, дескрипторы. Добавьте queries с section: runtime. process_* другого exporter и память контейнера не заменяют runtime API."},
}

func sectionIndex(id string) (int, bool) {
	for i, s := range sections {
		if s.ID == id {
			return i, true
		}
	}
	return 0, false
}

func querySection(m metric) string {
	if m.Section == "" {
		return "custom"
	}
	return m.Section
}

func (v *view) chooseSection(id string) error {
	i, ok := sectionIndex(id)
	if !ok {
		return errors.New("unknown section; use --sections")
	}
	v.section = i
	v.navFocus = false
	v.selected, v.seriesIndex, v.helpOffset = 0, 0, 0
	v.detail, v.breakdown, v.showHelp, v.showSections = false, false, false, false
	return nil
}

func (v view) sectionChoices() []int {
	var choices []int
	for i, s := range sections {
		if (s.ID == "custom" || s.ID == "runtime" || s.ID == "outbound") && i != v.section {
			copy := v
			copy.section = i
			if len(copy.rows()) == 0 {
				continue
			}
		}
		choices = append(choices, i)
	}
	return choices
}

func (v view) sectionOffset(from, delta int, cycle bool) int {
	choices := v.sectionChoices()
	at := max(0, slices.Index(choices, from)) + delta
	if cycle {
		at = (at%len(choices) + len(choices)) % len(choices)
	} else {
		at = max(0, min(len(choices)-1, at))
	}
	return choices[at]
}

// Returns true only when a section is confirmed; moving the cursor does not poll.
func (v *view) sectionKey(name, text string) bool {
	name = navigationKey(name, text)
	switch name {
	case "esc":
		v.showSections = false
	case "up":
		v.sectionCursor = v.sectionOffset(v.sectionCursor, -1, true)
	case "down":
		v.sectionCursor = v.sectionOffset(v.sectionCursor, 1, true)
	case "page-up":
		v.sectionCursor = v.sectionOffset(v.sectionCursor, -5, false)
	case "page-down":
		v.sectionCursor = v.sectionOffset(v.sectionCursor, 5, false)
	case "enter":
		v.chooseSection(sections[v.sectionCursor].ID)
		return true
	case "text":
		if text == "s" {
			v.showSections = false
		}
	}
	return false
}

func (v view) sectionMenu(width, budget int) []string {
	choices := v.sectionChoices()
	capacity := max(1, budget-3)
	start := max(0, slices.Index(choices, v.sectionCursor)-capacity+1)
	end := min(len(choices), start+capacity)
	lines := []string{"РАЗДЕЛЫ · ↑↓/jk · Enter открыть · Esc назад"}
	for _, i := range choices[start:end] {
		mark := "  "
		if i == v.sectionCursor {
			mark = "› "
		}
		copy := v
		copy.section = i
		note := fmt.Sprintf("%d показателей", len(copy.rows()))
		if len(copy.rows()) == 0 {
			note = "не настроено"
		}
		lines = append(lines, tui.Clip(mark+sections[i].Title+" · "+note, width))
	}
	lines = append(lines, fmt.Sprintf("%d–%d из %d · [ / ] — соседний раздел", start+1, end, len(choices)))
	return lines
}

func (v view) emptySection(width, budget int) []string {
	lines := []string{"Нет настроенных показателей."}
	if note := v.profile.SectionNotes[sections[v.section].ID]; note != "" {
		lines = append(lines, wrap(note, width)...)
	}
	lines = append(lines, wrap(sections[v.section].Setup, width)...)
	return lines[:min(len(lines), max(1, budget))]
}
