package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

func matches(text, query string) bool {
	text = strings.ToLower(Plain(text))
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}
func table(rows []string) []string {
	widths := []int{}
	for _, row := range rows {
		for column, value := range strings.Split(row, "\t") {
			value = Clean(value)
			for column >= len(widths) {
				widths = append(widths, 0)
			}
			widths[column] = max(widths[column], Width(value))
		}
	}
	result := make([]string, len(rows))
	for i, row := range rows {
		var cells []string
		values := strings.Split(row, "\t")
		for column, value := range values {
			value = Clean(value)
			if column < len(values)-1 {
				value = Pad(value, min(48, widths[column]))
			}
			cells = append(cells, value)
		}
		result[i] = strings.Join(cells, "  ")
	}
	return result
}

// Select возвращает исходные индексы. Метки только для показа и не становятся
// командами; поиск и мультивыбор не меняют порядок данных.
func Select(ctx context.Context, frame Frame, rows []string, query string, multi bool) ([]int, error) {
	indexes, _, err := selectRows(ctx, frame, rows, query, multi, nil)
	return indexes, err
}

// SelectKey как Select для одиночного выбора, но additionally возвращает имя
// нажатого действия ("enter" или одну из клавиш из actions) для текущей строки.
func SelectKey(ctx context.Context, frame Frame, rows []string, query string, actions []string) (int, string, error) {
	indexes, action, err := selectRows(ctx, frame, rows, query, false, actions)
	if err != nil {
		return -1, "", err
	}
	return indexes[0], action, nil
}

func selectRows(ctx context.Context, frame Frame, rows []string, query string, multi bool, actions []string) ([]int, string, error) {
	actionSet := map[string]bool{}
	for _, a := range actions {
		actionSet[a] = true
	}
	terminal, err := Open()
	if err != nil {
		return nil, "", err
	}
	defer terminal.Close()
	restore, err := Raw(terminal)
	if err != nil {
		return nil, "", err
	}
	defer restore()
	closeScreen := Screen(terminal)
	defer closeScreen()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	keys := Keys(ctx, terminal)
	painter := NewPainter(terminal)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	selected := map[int]bool{}
	cursor := 0
	last := ""
	display := table(rows)
	customFooter := frame.Footer
	introOffset := 0
	initialSelection := true
	searchOnly := map[int]bool{}
	for _, index := range frame.SearchOnly {
		searchOnly[index] = true
	}
	for {
		width, height := Size(terminal)
		visible := []int{}
		total := 0
		for i, row := range rows {
			if strings.TrimSpace(query) == "" && searchOnly[i] {
				continue
			}
			total++
			if matches(row, query) {
				visible = append(visible, i)
			}
		}
		if initialSelection {
			for position, index := range visible {
				if index == frame.InitialIndex {
					cursor = position
					break
				}
			}
			initialSelection = false
		}
		cursor = max(0, min(cursor, len(visible)-1))
		intro := runewidth.Wrap(CleanText(frame.Intro), max(1, width-1))
		parts := strings.Split(intro, "\n")
		introBudget := max(1, height-13)
		scrollIntro := len(parts) > introBudget
		if scrollIntro {
			introOffset = min(max(0, introOffset), len(parts)-introBudget)
			intro = strings.Join(parts[introOffset:introOffset+introBudget], "\n") + fmt.Sprintf("\nОписание %d–%d/%d · PgUp/PgDn", introOffset+1, introOffset+introBudget, len(parts))
		}
		introLines := 0
		if intro != "" {
			introLines = len(strings.Split(intro, "\n")) + 2
		}
		count := max(1, height-8-introLines)
		start := max(0, cursor-count+1)
		end := min(len(visible), start+count)
		var body strings.Builder
		if frame.Intro != "" {
			body.WriteString(intro + "\n\n")
		}
		fmt.Fprintf(&body, "Поиск › %s\n\n", Clean(query))
		if len(visible) == 0 {
			body.WriteString(Paint("Нет совпадений. Измените поиск.", Dim, ColorEnabled(terminal)) + "\n")
		}
		for pos := start; pos < end; pos++ {
			index := visible[pos]
			marker := "  "
			if multi {
				marker = "○ "
				if selected[index] {
					marker = "● "
				}
			}
			if pos == cursor {
				if multi {
					marker = "› " + marker
				} else {
					marker = "› "
				}
			}
			row := marker + display[index]
			if pos == cursor {
				row = Paint(row, Accent+Bold, ColorEnabled(terminal))
			}
			body.WriteString(row + "\n")
		}
		if frame.Action != "" {
			body.WriteString("\n" + frame.Action + "\n")
		}
		footer := fmt.Sprintf("%d/%d rows · ↑↓ / Ctrl+N/P move · type to search · Enter select · Esc back", len(visible), total)
		if multi {
			footer = fmt.Sprintf("%d selected · Space/Tab toggle · Ctrl+A all · Enter apply · Esc back", len(selected))
		}
		if customFooter != "" {
			footer = fmt.Sprintf("%d/%d · ", len(visible), total) + customFooter
			if multi {
				footer = fmt.Sprintf("Выбрано: %d · %d/%d · %s", len(selected), len(visible), total, customFooter)
			}
		}
		frame.Width, frame.Height, frame.Color = width, height, ColorEnabled(terminal)
		frame.Body = body.String()
		frame.Footer = footer
		rendered := frame.Render()
		if rendered != last {
			painter.Draw(frame)
			last = rendered
		}
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case key, ok := <-keys:
			if !ok {
				return nil, "", ErrCanceled
			}
			if actionSet[key.Name] && len(visible) > 0 {
				return []int{visible[cursor]}, key.Name, nil
			}
			switch key.Name {
			case "ctrl-o":
				if frame.Action != "" {
					return nil, "", ErrAction
				}
			case "esc", "ctrl-c", "ctrl-d":
				return nil, "", ErrCanceled
			case "up":
				cursor = max(0, cursor-1)
			case "down":
				cursor = min(len(visible)-1, cursor+1)
			case "page-up":
				if scrollIntro {
					introOffset = max(0, introOffset-introBudget)
				} else {
					cursor = max(0, cursor-count)
				}
			case "page-down":
				if scrollIntro {
					introOffset = min(len(parts)-introBudget, introOffset+introBudget)
				} else {
					cursor = min(len(visible)-1, cursor+count)
				}
			case "home":
				cursor = 0
			case "end":
				cursor = len(visible) - 1
			case "backspace":
				runes := []rune(query)
				if len(runes) > 0 {
					query = string(runes[:len(runes)-1])
					cursor = 0
				}
			case "ctrl-l":
				query = ""
				cursor = 0
			case "text", "paste":
				if key.Name == "text" && key.Text == " " && multi && len(visible) > 0 {
					index := visible[cursor]
					if selected[index] {
						delete(selected, index)
					} else {
						selected[index] = true
					}
				} else {
					query += Clean(key.Text)
					cursor = 0
				}
			case "tab":
				if multi && len(visible) > 0 {
					index := visible[cursor]
					if selected[index] {
						delete(selected, index)
					} else {
						selected[index] = true
					}
					cursor = min(len(visible)-1, cursor+1)
				}
			case "ctrl-a":
				if multi {
					all := true
					for _, index := range visible {
						if !selected[index] {
							all = false
						}
					}
					for _, index := range visible {
						if all {
							delete(selected, index)
						} else {
							selected[index] = true
						}
					}
				}
			case "enter":
				if len(visible) == 0 && (!multi || len(selected) == 0) {
					continue
				}
				if !multi {
					return []int{visible[cursor]}, "enter", nil
				}
				if len(selected) == 0 {
					selected[visible[cursor]] = true
				}
				result := []int{}
				for i := range rows {
					if selected[i] {
						result = append(result, i)
					}
				}
				return result, "enter", nil
			}
		}
	}
}

// SelectHint открывает список в режиме одноклавишных хинтов: строчная буква
// немедленно возвращает action="copy", заглавная — "open", '/' включает поиск.
func SelectHint(ctx context.Context, frame Frame, rows []string, hints []rune, fold func(rune) (rune, bool)) (int, string, error) {
	terminal, err := Open()
	if err != nil {
		return -1, "", err
	}
	defer terminal.Close()
	restore, err := Raw(terminal)
	if err != nil {
		return -1, "", err
	}
	defer restore()
	closeScreen := Screen(terminal)
	defer closeScreen()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	keys := Keys(ctx, terminal)
	painter := NewPainter(terminal)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	hintByRune := map[rune]int{}
	for i := 0; i < len(rows) && i < len(hints); i++ {
		hintByRune[hints[i]] = i
	}
	display := table(rows)
	searching := false
	query := ""
	cursor := 0
	last := ""
	for {
		width, height := Size(terminal)
		visible := []int{}
		for i, row := range rows {
			if !searching || matches(row, query) {
				visible = append(visible, i)
			}
		}
		cursor = max(0, min(cursor, len(visible)-1))
		count := max(1, height-8)
		start := max(0, cursor-count+1)
		end := min(len(visible), start+count)
		color := ColorEnabled(terminal)
		var body strings.Builder
		if searching {
			fmt.Fprintf(&body, "Поиск › %s\n\n", Clean(query))
		} else {
			body.WriteString(Paint("Хинт › буква = копировать · Shift+буква = открыть · / = поиск", Dim, color) + "\n\n")
		}
		if len(visible) == 0 {
			body.WriteString(Paint("Нет совпадений. Измените поиск.", Dim, color) + "\n")
		}
		for pos := start; pos < end; pos++ {
			index := visible[pos]
			badge := "    "
			if !searching && index < len(hints) {
				tag := fmt.Sprintf("[%c] ", hints[index])
				if pos == cursor {
					badge = tag
				} else {
					badge = Paint(tag, Green+Bold, color)
				}
			}
			marker := "  "
			if pos == cursor {
				marker = "› "
			}
			row := marker + badge + display[index]
			if pos == cursor {
				row = Paint(marker+fmt.Sprintf("%-4s", strings.TrimRight(Plain(badge), " "))+display[index], Accent+Bold, color)
			}
			body.WriteString(row + "\n")
		}
		footer := fmt.Sprintf("%d/%d · буква copy · Shift+буква open · / поиск · Enter copy · Ctrl+O open · Esc", len(visible), len(rows))
		if searching {
			footer = fmt.Sprintf("%d/%d · ввод фильтр · Enter copy · Ctrl+O open · Esc хинты", len(visible), len(rows))
		}
		frame.Width, frame.Height, frame.Color = width, height, color
		frame.Body = body.String()
		frame.Footer = footer
		rendered := frame.Render()
		if rendered != last {
			painter.Draw(frame)
			last = rendered
		}
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return -1, "", ctx.Err()
		case key, ok := <-keys:
			if !ok {
				return -1, "", ErrCanceled
			}
			switch key.Name {
			case "esc":
				if searching {
					searching = false
					query = ""
					cursor = 0
					continue
				}
				return -1, "", ErrCanceled
			case "ctrl-c", "ctrl-d":
				return -1, "", ErrCanceled
			case "up":
				cursor = max(0, cursor-1)
			case "down":
				cursor = min(len(visible)-1, cursor+1)
			case "page-up":
				cursor = max(0, cursor-count)
			case "page-down":
				cursor = min(len(visible)-1, cursor+count)
			case "home":
				cursor = 0
			case "end":
				cursor = len(visible) - 1
			case "backspace":
				if searching {
					runes := []rune(query)
					if len(runes) > 0 {
						query = string(runes[:len(runes)-1])
						cursor = 0
					}
				}
			case "ctrl-l":
				query = ""
				cursor = 0
			case "enter":
				if len(visible) > 0 {
					return visible[cursor], "copy", nil
				}
			case "ctrl-o":
				if len(visible) > 0 {
					return visible[cursor], "open", nil
				}
			case "paste":
				if searching {
					query += Clean(key.Text)
					cursor = 0
				}
			case "text":
				if searching {
					query += Clean(key.Text)
					cursor = 0
					continue
				}
				if key.Text == "/" || key.Text == "." {
					searching = true
					query = ""
					cursor = 0
					continue
				}
				runes := []rune(key.Text)
				if len(runes) == 1 && fold != nil {
					if letter, upper := fold(runes[0]); letter != 0 {
						if index, ok := hintByRune[letter]; ok {
							if upper {
								return index, "open", nil
							}
							return index, "copy", nil
						}
					}
				}
			}
		}
	}
}
func Input(ctx context.Context, frame Frame, value string) (string, error) {
	return input(ctx, frame, value, false)
}

// SecretInput не показывает значение, включая начальное и вставленное.
func SecretInput(ctx context.Context, frame Frame, value string) (string, error) {
	return input(ctx, frame, value, true)
}

func input(ctx context.Context, frame Frame, value string, secret bool) (string, error) {
	terminal, err := Open()
	if err != nil {
		return "", err
	}
	defer terminal.Close()
	restore, err := Raw(terminal)
	if err != nil {
		return "", err
	}
	defer restore()
	end := Screen(terminal)
	defer end()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	keys := Keys(ctx, terminal)
	painter := NewPainter(terminal)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		width, height := Size(terminal)
		frame.Width, frame.Height, frame.Color = width, height, ColorEnabled(terminal)
		display := Clean(value)
		if secret {
			display = strings.Repeat("•", len([]rune(value)))
		}
		intro := strings.Split(runewidth.Wrap(CleanText(frame.Intro), max(1, width-1)), "\n")
		budget := max(0, height-8)
		if len(intro) > budget {
			intro = append(intro[:max(0, budget-1)], "… описание сокращено")
		}
		frame.Body = strings.Join(intro, "\n") + "\n\n" + Clean(frame.Prompt) + " › " + Clip(display, max(1, width-Width(frame.Prompt)-5)) + "█\n"
		if frame.Action != "" {
			frame.Body += "\n" + frame.Action + "\n"
		}
		if frame.Footer == "" {
			frame.Footer = "Enter — продолжить · Ctrl+L — очистить · Esc — назад"
		}
		painter.Draw(frame)
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return "", ctx.Err()
		case key, ok := <-keys:
			if !ok {
				return "", ErrCanceled
			}
			switch key.Name {
			case "ctrl-o":
				if frame.Action != "" {
					return value, ErrAction
				}
			case "esc", "ctrl-c", "ctrl-d":
				return "", ErrCanceled
			case "enter":
				return value, nil
			case "ctrl-l":
				value = ""
			case "backspace":
				runes := []rune(value)
				if len(runes) > 0 {
					value = string(runes[:len(runes)-1])
				}
			case "text", "paste":
				value += Clean(key.Text)
			}
		}
	}
}
