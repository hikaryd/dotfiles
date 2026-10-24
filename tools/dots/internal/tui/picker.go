package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

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

// Select returns original row indexes. Labels are display-only and cannot turn
// into commands; search and multi-selection never reorder the caller's data.
func Select(ctx context.Context, frame Frame, rows []string, query string, multi bool) ([]int, error) {
	terminal, err := Open()
	if err != nil {
		return nil, err
	}
	defer terminal.Close()
	restore, err := Raw(terminal)
	if err != nil {
		return nil, err
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
			return nil, ctx.Err()
		case key, ok := <-keys:
			if !ok {
				return nil, ErrCanceled
			}
			switch key.Name {
			case "ctrl-o":
				if frame.Action != "" {
					return nil, ErrAction
				}
			case "esc", "ctrl-c", "ctrl-d":
				return nil, ErrCanceled
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
					return []int{visible[cursor]}, nil
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
				return result, nil
			}
		}
	}
}
func Input(ctx context.Context, frame Frame, value string) (string, error) {
	return input(ctx, frame, value, false)
}

// SecretInput never renders the value, including initial values and pasted text.
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
func Editor(ctx context.Context, frame Frame) (string, error) {
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
	value := ""
	for {
		width, height := Size(terminal)
		frame.Width, frame.Height, frame.Color = width, height, ColorEnabled(terminal)
		lines := strings.Split(value, "\n")
		start := max(0, len(lines)-max(1, height-6))
		var body strings.Builder
		for i := start; i < len(lines); i++ {
			line := fmt.Sprintf("%3d  %s", i+1, Clean(lines[i]))
			if i == len(lines)-1 {
				line += "█"
			}
			body.WriteString(line + "\n")
		}
		frame.Body = body.String()
		frame.Footer = "Paste multiline code · Enter newline · Ctrl+D run · Esc cancel"
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
			case "esc", "ctrl-c":
				return "", ErrCanceled
			case "ctrl-d":
				return value, nil
			case "enter":
				value += "\n"
			case "backspace":
				runes := []rune(value)
				if len(runes) > 0 {
					value = string(runes[:len(runes)-1])
				}
			case "text", "paste":
				value += strings.Map(func(r rune) rune {
					if r == '\r' {
						return '\n'
					}
					if r == '\n' || r == '\t' || !unicode.IsControl(r) {
						return r
					}
					return -1
				}, Plain(key.Text))
			}
		}
	}
}
