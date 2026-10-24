package metrics

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"dots/native/internal/tui"
)

type pickOption struct{ ID, Title, Hint string }
type picker struct {
	Kind, Query string
	Error       string
	Selected    int
	Options     []pickOption
}

func (p picker) matches() []pickOption {
	if p.Query == "" {
		return p.Options
	}
	var rows []pickOption
	needle := strings.ToLower(p.Query)
	for _, row := range p.Options {
		if strings.Contains(strings.ToLower(row.Title+" "+row.ID+" "+row.Hint), needle) {
			rows = append(rows, row)
		}
	}
	return rows
}

func (v *view) openPicker(kind string) {
	p := picker{Kind: kind}
	switch kind {
	case "route":
		p.Query = v.profile.RouteFilter
	case "services":
		p.Options = append(p.Options, pickOption{ID: "", Title: "Все сервисы", Hint: "общий обзор namespace"})
		for _, name := range v.services {
			p.Options = append(p.Options, pickOption{ID: name, Title: name, Hint: "HTTP и явно привязанные запросы"})
			if name == v.profile.Service {
				p.Selected = len(p.Options) - 1
			}
		}
	case "metrics":
		for _, m := range v.items {
			p.Options = append(p.Options, pickOption{ID: m.ID, Title: v.displayMetricTitle(m), Hint: v.metricSection(m) + " · " + m.ID})
		}
	}
	v.picker = p
	v.showHelp = false
	v.showSections = false
	v.navFocus = false
}

func (v view) displayMetricTitle(m metric) string {
	title := tui.Clean(m.Title)
	if v.profile.Service == "" {
		return title
	}
	for _, own := range v.profile.Queries {
		if own.ID == m.ID && own.ServiceQuery == "" {
			return "[фикс.] " + title
		}
	}
	for _, section := range sections[2:4] {
		for _, id := range section.Metrics {
			if id == m.ID || breakdowns[id] == m.ID {
				return "[общие] " + strings.TrimPrefix(title, "Использование ")
			}
		}
	}
	return title
}

func (v view) metricSection(m metric) string {
	if m.Section != "" {
		if i, ok := sectionIndex(m.Section); ok {
			return sections[i].Title
		}
	}
	for _, section := range sections[1:] {
		for _, id := range section.Metrics {
			if id == m.ID || breakdowns[id] == m.ID || companions[id] == m.ID {
				return section.Title
			}
		}
	}
	return "Свои"
}

// Modal text input owns printable keys, including q, c, s and spaces. Pasted
// text is ignored by the shared key decoder contract, never interpreted as keys.
func (v *view) pickerKey(name, text string) bool {
	p := &v.picker
	rows := p.matches()
	switch name {
	case "esc":
		v.picker = picker{}
	case "up":
		if len(rows) > 0 {
			p.Selected = (p.Selected - 1 + len(rows)) % len(rows)
		}
	case "down":
		if len(rows) > 0 {
			p.Selected = (p.Selected + 1) % len(rows)
		}
	case "page-up":
		p.Selected = max(0, p.Selected-8)
	case "page-down":
		p.Selected = min(max(0, len(rows)-1), p.Selected+8)
	case "ctrl-l":
		p.Query = ""
		p.Selected = 0
		p.Error = ""
	case "backspace":
		p.Error = ""
		if p.Query != "" {
			_, size := utf8.DecodeLastRuneInString(p.Query)
			p.Query = p.Query[:len(p.Query)-size]
			p.Selected = 0
		}
	case "text":
		p.Error = ""
		limit := 256
		if p.Kind == "route" {
			limit = 512
		}
		if len(p.Query)+len(text) <= limit {
			p.Query += text
			p.Selected = 0
		}
	case "enter":
		if p.Kind == "route" {
			if validateRouteFilter(p.Query) != nil {
				p.Error = "Некорректное регулярное выражение. Исправьте или нажмите Esc."
				return false
			}
			v.profile.RouteFilter = p.Query
			v.items = v.profile.metrics()
			v.seriesIndex = 0
			for _, m := range routeMetrics(v.profile) {
				delete(v.readings, m.ID)
			}
			v.picker = picker{}
			return true
		}
		if len(rows) == 0 {
			return false
		}
		choice := rows[min(p.Selected, len(rows)-1)]
		kind := p.Kind
		v.picker = picker{}
		if kind == "services" {
			v.setService(choice.ID)
			return true
		}
		if kind == "metrics" {
			v.chooseMetric(choice.ID)
			return true
		}
	}
	return false
}

func (v view) pickerLines(width, budget int) []string {
	if v.picker.Kind == "route" {
		lines := []string{"ФИЛЬТР РУЧЕК", "RE2 regex шаблона маршрута, например /items.*", "Пустой фильтр — все ручки; ограничение топ-10 остаётся.", inputLine("Фильтр: ", v.picker.Query, width)}
		if v.picker.Error != "" {
			lines = append(lines, v.picker.Error)
		}
		return append(lines, "Enter применить · Ctrl+L очистить · Esc отменить")
	}
	title := "ВСЕ МЕТРИКИ"
	if v.picker.Kind == "services" {
		title = "СЕРВИСЫ"
	}
	lines := []string{title + " · печатайте для поиска", inputLine("Поиск: ", v.picker.Query, width)}
	if v.picker.Kind == "services" {
		if !v.servicesLoaded {
			lines = append(lines, "Получаем сервисы из HTTP-метрик текущего namespace…")
		}
		if v.serviceError != "" {
			lines = append(lines, tui.Clip(v.serviceError, width))
		}
	}
	rows := v.picker.matches()
	if len(rows) == 0 {
		return append(lines, "Совпадений нет. Backspace — исправить, Esc — назад.")
	}
	capacity := max(1, budget-len(lines)-1)
	selected := min(v.picker.Selected, len(rows)-1)
	start := max(0, selected-capacity+1)
	end := min(len(rows), start+capacity)
	for i := start; i < end; i++ {
		mark := "  "
		if i == selected {
			mark = "› "
		}
		line := mark + rows[i].Title
		if width >= 85 {
			line = tui.Pad(tui.Clip(line, width/2), width/2) + rows[i].Hint
		}
		lines = append(lines, tui.Clip(line, width))
	}
	return append(lines, fmt.Sprintf("%d–%d из %d · Enter выбрать · Esc назад", start+1, end, len(rows)))
}

func inputLine(label, value string, width int) string {
	value = tui.Clean(value)
	available := max(1, width-tui.Width(label)-1)
	if tui.Width(value) > available {
		for tui.Width(value) > max(0, available-1) && value != "" {
			_, size := utf8.DecodeRuneInString(value)
			value = value[size:]
		}
		value = "…" + value
	}
	return label + value + "▏"
}

// Normalize only outside text editors. Search and route input own all letters.
func navigationKey(name, text string) string {
	if name == "text" {
		if text == "j" {
			return "down"
		}
		if text == "k" {
			return "up"
		}
	}
	return name
}

func (v *view) confirmNav() {
	if v.navCursor != v.section {
		v.chooseSection(sections[v.navCursor].ID)
	}
	v.navFocus = false
}

func (v view) sidebar(height int, color bool) []string {
	title := "РАЗДЕЛЫ · Tab"
	if v.navFocus {
		title = "РАЗДЕЛЫ · ↑↓/jk Enter"
	}
	lines := []string{tui.Paint(title, tui.Dim, color)}
	cursor := v.section
	if v.navFocus {
		cursor = v.navCursor
	}
	choices := v.sectionChoices()
	capacity := max(1, height-4)
	start := max(0, slices.Index(choices, cursor)-capacity+1)
	end := min(len(choices), start+capacity)
	for _, i := range choices[start:end] {
		mark, tone := "  ", tui.Dim
		if i == v.section {
			mark, tone = "• ", tui.Text
		}
		if v.navFocus && i == cursor {
			mark, tone = "› ", tui.Accent+tui.Bold
		}
		lines = append(lines, tui.Paint(mark+sections[i].Title, tone, color))
	}
	hint := "Tab — выбрать раздел"
	if v.navFocus {
		hint = "Tab — к показателям"
	}
	lines = append(lines, "", hint)
	return lines[:min(len(lines), height)]
}

func columns(left, right []string, leftWidth, width int, color bool) []string {
	height := max(len(left), len(right))
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, tui.Pad(tui.Clip(l, leftWidth), leftWidth)+tui.Paint(" │ ", tui.Border, color)+tui.Clip(r, width-leftWidth-3))
	}
	return out
}
