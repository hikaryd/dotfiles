package metrics

import (
	"fmt"
	"strings"
	"time"

	"dots/native/internal/tui"
)

func (v view) state(r reading) string {
	if r.Error != "" || v.err != "" {
		if r.Data != nil {
			return "stale"
		}
		return "error"
	}
	if r.Data == nil {
		if v.loading {
			return "loading"
		}
		return "not-loaded"
	}
	if len(r.Data.Series) == 0 {
		return "empty"
	}
	finite, bounded := false, false
	for _, s := range r.Data.Series {
		if current(s) != nil {
			finite = true
		}
		if s.Current != nil && s.Current.Reason == "above-buckets" {
			bounded = true
		}
	}
	if !finite && !bounded {
		return "undefined"
	}
	if v.paused {
		return "paused"
	}
	if v.now.Sub(r.Data.FetchedAt) > v.maxAge {
		return "old"
	}
	if bounded {
		return "bounded"
	}
	return "ready"
}
func (v view) status(r reading) string {
	switch v.state(r) {
	case "stale":
		return "старые данные"
	case "error":
		return "ошибка запроса"
	case "loading":
		return "загрузка…"
	case "not-loaded":
		return "не загружено"
	case "empty":
		return "нет измерений"
	case "undefined":
		return "нет значения"
	case "bounded":
		return "за границей"
	case "paused":
		return "пауза"
	case "old":
		return "давно: " + durationText(v.now.Sub(r.Data.FetchedAt))
	default:
		return durationText(v.now.Sub(r.Data.FetchedAt)) + " назад"
	}
}
func (v view) statusTone(r reading) string {
	switch v.state(r) {
	case "stale", "error":
		return tui.Red
	case "old", "bounded":
		return tui.Yellow
	default:
		return tui.Dim
	}
}

func (v view) frame(width, height int, color bool) tui.Frame {
	width, height = max(20, width), max(8, height)
	mode := "Наблюдение"
	if v.paused {
		mode = "ПАУЗА"
	} else if v.loading {
		mode = "Обновление…"
	}
	if v.err != "" {
		mode = "Нет соединения"
	}
	title := v.profile.Name
	if v.detail {
		title += " · " + v.focused().Title
	} else {
		title += " · " + sections[v.section].Title
	}
	service := tui.Clean(v.profile.Service)
	if service == "" {
		service = "Все"
	}
	hint := " · c выбрать · / все метрики"
	if width < 60 {
		hint = " · c"
	}
	serviceLine := tui.Clip("Сервис: "+service, max(10, width-tui.Width(hint))) + hint
	sectionLabel := sections[v.section].Title
	for i, id := range sectionShortcuts {
		if sections[v.section].ID == id {
			sectionLabel = fmt.Sprintf("%d %s", i+1, sectionLabel)
		}
	}
	sectionHint := ""
	if width < 120 {
		sectionHint = " · s разделы"
	}
	if sections[v.section].ID == "routes" {
		sectionHint += " · топ-10 · f фильтр"
	}
	body := []string{serviceLine, "[" + sectionLabel + "]" + sectionHint}
	footer := "Tab разделы · ↑↓/jk выбор · Enter график · ←→ период · c сервис · / поиск · ? · q выход"
	if v.detail {
		footer = "↑↓/jk ряд · ←→ период · Esc обзор · c сервис · / поиск · ? · q выход"
	}
	if width < 100 {
		footer = "↑↓/jk · Enter · c сервис · / поиск · ? · q выход"
	}
	if width < 60 {
		footer = "c сервис · / метрики · ? · q выход"
	}
	if v.navFocus {
		footer = "↑↓/jk раздел · Enter открыть · Tab к показателям · q выход"
	}
	budget := max(1, height-len(body)-5)
	if v.picker.Kind != "" {
		body = append(body, v.pickerLines(width, budget)...)
		footer = "↑↓ · Enter выбрать · Esc назад · Ctrl+C выход"
		if width < 60 {
			footer = "↑↓ · Enter · Esc · Ctrl+C выход"
		}
	} else if v.showSections {
		body = append(body, v.sectionMenu(width, budget)...)
		footer = "↑↓/jk · Enter открыть · Esc · q выход"
	} else if v.showHelp {
		lines := v.helpLines(width)
		capacity := max(1, budget-1)
		start := min(v.helpOffset, max(0, len(lines)-capacity))
		body = append(body, fmt.Sprintf("Справка %d–%d/%d · ↑↓ / PgUp PgDn · ? закрыть", start+1, min(len(lines), start+capacity), len(lines)))
		body = append(body, lines[start:min(len(lines), start+capacity)]...)
	} else {
		mainWidth := width
		if width >= 120 {
			mainWidth -= 31
		}
		scope := v.scopeLabel(v.focused().ID)
		if scope == "область задана запросом" {
			scope = "Область источника задана формулой · ? подробности"
		}
		var main []string
		if v.detail {
			main = append([]string{tui.Clip(scope, mainWidth)}, v.detailLines(mainWidth, max(1, budget-1), color)...)
		} else {
			main = v.overviewLines(mainWidth, budget, color)
		}
		if width >= 120 {
			body = append(body, columns(v.sidebar(budget, color), main, 28, width, color)...)
		} else {
			body = append(body, main...)
		}
	}
	step := v.step
	if v.detail {
		if data := v.readings[v.focused().ID].Data; data != nil {
			step = time.Duration(data.Step) * time.Second
		}
	}
	return tui.Frame{Brand: "dots metrics", Title: title, Mode: mode, Tone: tui.Accent,
		Subtitle: "Последние " + durationText(v.period) + " · шаг графика " + durationText(step) + " · только чтение",
		Body:     strings.Join(body, "\n"), Footer: footer, Width: width, Height: height, Color: color}
}

func (v view) scopeLabel(id string) string {
	if v.profile.Service != "" {
		for _, m := range v.profile.Queries {
			if m.ID == id && m.ServiceQuery == "" {
				return "Фиксированная область, не фильтр сервиса · " + tui.Clean(m.Scope)
			}
		}
	}
	if i, err := findMetric(v.items, id); err == nil && v.items[i].Scope != "" {
		return tui.Clean(v.items[i].Scope)
	}
	if v.profile.CustomOnly {
		return "область задана запросом"
	}
	switch id {
	case "rps", "rpm", "errors5xx", "errors5xxrate", "errors4xx", "errors4xxrate", "p95", "p99", "statuses", "services", "errorservices", "latencyservices", "latencyservices99":
		if v.profile.Service != "" {
			return "HTTP: " + tui.Clean(v.profile.Service)
		}
		return "HTTP: все сервисы"
	case "cpurequests", "cpulimits", "memoryrequests", "memorylimits":
		return "только контейнеры с заданным резервом/лимитом"
	case "cpu", "cpupods", "memory", "memorypods", "throttling", "unavailable", "workloads", "notready", "pending", "restarts", "restartpods", "scrapes":
		return "Namespace: " + tui.Clean(v.profile.Namespace)
	default:
		return "область задана запросом"
	}
}

func (v view) observation() string {
	if v.err != "" {
		return "Нет соединения. Сохранённые значения не актуальны. r — подключиться заново."
	}
	if v.paused {
		return "Обновление остановлено. Пробел — продолжить."
	}
	visible := map[string]bool{}
	for _, m := range v.rows() {
		visible[m.ID] = true
	}
	for _, id := range []string{"unavailable", "notready", "restarts", "scrapes", "errors5xx"} {
		r := v.readings[id]
		if !visible[id] || v.state(r) != "ready" {
			continue
		}
		if n := single(r); n != nil && *n > 0 {
			switch id {
			case "unavailable":
				return "Есть недоступные реплики. Выберите строку → Enter → b: какие Deployment."
			case "notready":
				return "Часть подов ещё не готова принимать трафик. Проверьте выкладку и события."
			case "restarts":
				return "За последние 15 минут были перезапуски. Enter → b: какие поды."
			case "scrapes":
				return "Часть источников не отдаёт метрики. Данные приложения могут быть неполными."
			case "errors5xx":
				return "Есть ответы 5xx. Сравните частоту ошибок, нагрузку и время ответа."
			}
		}
	}
	return ""
}

func (v view) breakdownHint() string {
	if v.profile.CustomOnly {
		return ""
	}
	if v.breakdown {
		return "b — общий график"
	}
	switch breakdowns[v.focused().ID] {
	case "services", "errorservices", "latencyservices", "latencyservices99":
		return "b — по сервисам"
	case "cpupods", "memorypods", "restartpods":
		return "b — по подам"
	case "workloads":
		return "b — по Deployment"
	default:
		return ""
	}
}

func (v view) detailLines(width, budget int, color bool) []string {
	m := v.focused()
	r := v.readings[m.ID]
	rows := ranked(r.Data)
	lines := []string{}
	if v.state(r) == "stale" && budget >= 10 {
		lines = append(lines, tui.Paint("Не удалось обновить: старые значения. r — повторить; ? — причина.", tui.Red, color))
	} else if v.state(r) == "error" {
		lines = append(lines, tui.Paint("Не удалось получить данные. r — повторить; ? — причина.", tui.Red, color))
	}
	if len(rows) == 0 {
		message := "Нет измерений для этой метрики. Это не ноль. Проверьте источник и фильтры."
		if r.Data == nil {
			message = "Загружаем данные… q — выйти, не дожидаясь ответа."
			if v.state(r) == "error" {
				message = "Нет ответа от мониторинга. r — повторить подключение."
			}
		}
		lines = append(lines, wrap(message, width)...)
		lines = append(lines, wrap(m.Description, width)...)
		return lines[:min(len(lines), budget)]
	}
	index := min(v.seriesIndex, len(rows)-1)
	s := rows[index]
	currentLabel := "Сейчас"
	if v.state(r) == "stale" || v.state(r) == "old" {
		currentLabel = "Последнее значение"
	} else if v.paused {
		currentLabel = "На паузе"
	}
	currentLine := currentLabel + "  " + pointText(s.Current, m.Unit)
	if width >= 90 && current(s) != nil && !v.profile.CustomOnly && m.Unit == "req/s" {
		perMinute := *current(s) * 60
		currentLine += "  ≈ " + number(&perMinute) + " запр./мин"
	}
	if width >= 100 && (v.state(r) == "ready" || v.state(r) == "paused") {
		if change := changeText(s, r.Data, m.Unit); change != "" {
			currentLine += "   " + change
		}
	}
	lines = append(lines, tui.Paint(currentLine, tui.Accent+tui.Bold, color))
	if note := quantileNote(s.Current, m.Unit); note != "" && (budget >= 10 || s.Current.Reason != "estimate") {
		lines = append(lines, tui.Clip(note, width))
	}
	if note := sampleNote(s.Current, r.Data.Quantile); note != "" && budget >= 12 {
		lines = append(lines, tui.Clip(note, width))
	}
	censored := 0
	for _, p := range s.Points {
		if p.Reason == "above-buckets" {
			censored++
		}
	}
	if censored > 0 && budget >= 14 {
		lines = append(lines, tui.Clip(fmt.Sprintf("Точек за границей: %d · разрывы, не точные значения; порядок таких ручек неизвестен.", censored), width))
	}
	low, mean, high, count := statistics(s.Points, r.Data.Start, r.Data.End)
	if count > 0 && (budget >= 10 || (budget >= 9 && len(rows) == 1)) {
		scale := scaleFor(m.Unit, maxAbs(low, high))
		prefix := "За период: минимум "
		if censored > 0 {
			prefix = "По вычислимым точкам: минимум "
		}
		stats := prefix + scale.format(low) + " · среднее " + scale.format(mean) + " · пик " + scale.format(high) + " " + scale.unit
		lines = append(lines, tui.Clip(stats, width))
	}
	if len(s.Labels) > 0 {
		prefix := fmt.Sprintf("%d/%d  ", index+1, len(rows))
		if v.profile.RouteLabel != "" && s.Labels[v.profile.RouteLabel] != "" {
			lines = append(lines, prefix+v.sourceCell(s.Labels, width-tui.Width(prefix), color, true, true))
		} else {
			lines = append(lines, prefix+tui.Clean(seriesLabel(s.Labels)))
		}
	}
	if id := companions[m.ID]; id != "" && !v.profile.CustomOnly && budget >= 14 {
		pair := v.readings[id]
		lines = append(lines, "В абсолютных числах: "+valueText(single(pair), "req/s")+" · "+v.status(pair))
	}
	// Allocate the remaining HEIGHT to the plot, not a hard-coded eight rows.
	// Reserve 3 rows for axes/unit, 1 for the hint, and a bounded ranked table.
	tableRows := 0
	if len(rows) > 1 && budget-len(lines) >= 10 {
		tableRows = min(6, max(2, (budget-len(lines))/4))
	}
	chartRows := max(2, budget-len(lines)-3-1-tableRows)
	lines = append(lines, historyChart(s, r.Data, m.Unit, width, chartRows, color)...)
	if len(rows) > 1 {
		tableRows = min(len(rows)+1, max(0, budget-len(lines)-1))
	}
	if tableRows > 1 {
		caption := "↑↓/jk — ряд · по текущему значению"
		if r.Data.Quantile > 0 {
			caption = "↑↓/jk — ряд · по текущей оценке"
		}
		for _, row := range rows {
			if row.Current != nil && row.Current.Reason == "above-buckets" {
				caption = "↑↓/jk — ряд · порядок за границей неизвестен"
				break
			}
		}
		lines = append(lines, caption)
		capacity := tableRows - 1
		start := max(0, index-capacity+1)
		for i := start; i < min(len(rows), start+capacity); i++ {
			mark := "  "
			if i == index && !v.navFocus {
				mark = "› "
			}
			prefix := mark + tui.Pad(pointText(rows[i].Current, m.Unit), 22)
			active := i == index && !v.navFocus
			tone := tui.Text
			if active {
				tone = tui.Accent + tui.Bold
			}
			line := tui.Paint(prefix, tone, color) + v.sourceCell(rows[i].Labels, max(1, width-tui.Width(prefix)), color, active, true)
			lines = append(lines, line)
		}
	}
	hint := "Обновлено " + r.Data.FetchedAt.Local().Format("15:04:05") + " · " + v.status(r)
	if text := v.breakdownHint(); text != "" {
		hint += " · " + text
	}
	lines = append(lines, hint)
	return lines
}

func maxAbs(a, b float64) float64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	return max(a, b)
}

func (v view) helpLines(width int) []string {
	lines := []string{
		"УПРАВЛЕНИЕ",
		"Один km: c — сервис; / — поиск по всем метрикам; Tab — боковая навигация (на узком экране меню). Enter — открыть.",
		"s — разделы; [ / ] — соседний; 1–5 — Обзор, HTTP, Ресурсы, Состояние, Ручки. ↑↓/jk — показатель или ряд. f в ручках — фильтр маршрута, Ctrl+L очищает поиск/фильтр.",
		"←→ — последние 15 мин / 1 ч / 6 ч / 24 ч. b — разбивка; p — следующий источник (приватный профиль).",
		"Пробел — пауза; r — подключиться заново; Esc — назад; q/Ctrl+C — выход; ? — закрыть справку.",
		"",
		"КАК ЧИТАТЬ ДАННЫЕ",
		"Ноль — измеренный ноль. «Нет измерений» — подходящих рядов нет. «Нет значения» — результат не определён, например 0/0.",
		"«Старые данные» — запрос не удался, показан прошлый снимок. Время обновления — ответ API, не время исходного сбора.",
		"Линия соединяет только соседние измерения. Если точка пропущена или не определена, линия разрывается, а не идёт к нулю.",
		"Среднее на экране — по доступным точкам периода. Мини-графики имеют разные масштабы; их высоты между строками несравнимы.",
		"Обновление над таблицей — возраст самого старого ответа API. Ошибки и устаревшие данные дополнительно отмечены в строках.",
		"Δ — изменение к началу периода; · — без изменений. Без измерения в начале Δ не вычисляется.",
		"Методы: GET — зелёный, POST — голубой, PUT — жёлтый, PATCH — фиолетовый, DELETE — красный. Буквы j/k в поиске и фильтре вводятся как текст.",
		"[общие] — ресурсы/готовность всего namespace. [фикс.] — собственный фильтр запроса. Выбор сервиса не меняет эти источники.",
		"≈ — интерполяция внутри bucket, не точное время. ≥ — только нижняя граница; такие точки не участвуют в графике и статистике оценок.",
		"Малое число запросов делает хвостовые перцентили нестабильными. Порядок ручек за последней границей неизвестен.",
		"Сервисы берутся из HTTP-метрик namespace. Не все общие показатели привязаны к сервису: фиксированная область подписана явно.",
		"Формулы не определяют причину аварии. CPU выше резерва не означает насыщение; перезапуски не доказывают OOM.",
	}
	if note := v.profile.SectionNotes[sections[v.section].ID]; note != "" {
		lines = append(lines, "", "ПОКРЫТИЕ РАЗДЕЛА", note)
	}
	if setup := sections[v.section].Setup; setup != "" {
		lines = append(lines, "", setup)
	}
	m := v.focused()
	if m.ID != "" {
		lines = append(lines, "", tui.Clean(m.Title)+" · "+v.scopeLabel(m.ID), m.Description)
		rows := ranked(v.readings[m.ID].Data)
		if len(rows) > 0 {
			lines = append(lines, "Ряд: "+seriesLabel(rows[min(v.seriesIndex, len(rows)-1)].Labels))
		}
		lines = append(lines, "PromQL: "+m.Query)
		if m.Histogram != nil {
			lines = append(lines, "Источник оценки — buckets: "+m.Histogram.Query)
		}
		if r := v.readings[m.ID]; r.Error != "" {
			lines = append(lines, "Ошибка (ответ сервера скрыт): "+r.Error)
		}
	}
	if v.err != "" {
		lines = append(lines, "Ошибка подключения: "+v.err)
	}
	return wrap(strings.Join(lines, "\n"), width)
}

// Preserve whitespace inside PromQL string literals, including long selectors.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, r := range tui.CleanText(text) {
		if r == '\n' {
			lines = append(lines, line)
			line = ""
			continue
		}
		if line != "" && tui.Width(line+string(r)) > width {
			lines = append(lines, line)
			line = ""
		}
		line += string(r)
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
