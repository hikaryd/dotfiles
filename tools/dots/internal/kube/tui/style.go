// Package tui owns the shared Ayu presentation and terminal lifecycle of the
// Kubernetes tools. Its renderers are pure; raw terminal state is always restored.
package tui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const (
	Reset     = "\x1b[0m"
	Bold      = "\x1b[1m"
	Dim       = "\x1b[38;2;101;117;139m"
	Text      = "\x1b[38;2;191;189;182m"
	Accent    = "\x1b[38;2;115;208;255m"
	Green     = "\x1b[38;2;149;230;203m"
	Yellow    = "\x1b[38;2;230;180;80m"
	Red       = "\x1b[38;2;240;113;120m"
	Purple    = "\x1b[38;2;212;191;255m"
	Border    = "\x1b[38;2;39;55;71m"
	Highlight = "\x1b[30;48;2;230;180;80m"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

func Plain(value string) string { return ansi.ReplaceAllString(value, "") }
func Clean(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 127 {
			return -1
		}
		return r
	}, Plain(value))
}

// CleanText preserves meaningful line breaks in diagnostics while removing
// terminal escape sequences and other control characters.
func CleanText(value string) string {
	parts := strings.Split(value, "\n")
	for i := range parts {
		parts[i] = Clean(parts[i])
	}
	return strings.Join(parts, "\n")
}
func Width(value string) int { return runewidth.StringWidth(Plain(value)) }
func Clip(value string, width int) string {
	if width < 1 {
		return ""
	}
	if Width(value) <= width {
		return value
	}
	return runewidth.Truncate(Plain(value), width, "…")
}
func Pad(value string, width int) string {
	value = Clip(value, width)
	return value + strings.Repeat(" ", max(0, width-Width(value)))
}
func IsTerminal(file *os.File) bool { return term.IsTerminal(int(file.Fd())) }
func ColorEnabled(file *os.File) bool {
	_, disabled := os.LookupEnv("NO_COLOR")
	return !disabled && os.Getenv("TERM") != "dumb" && term.IsTerminal(int(file.Fd()))
}
func Paint(value, tone string, enabled bool) string {
	if !enabled {
		return Plain(value)
	}
	return tone + value + Reset
}
func Size(file *os.File) (int, int) {
	width, height, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 100, 28
	}
	return max(20, width), max(8, height)
}

type Frame struct {
	Title, Environment, Context, Namespace, Mode, Body, Footer string
	Intro, Prompt, Action                                      string
	Width, Height                                              int
	Color                                                      bool
	// InitialIndex is the original row index highlighted when Select opens.
	InitialIndex int
}

func (f Frame) Header() string {
	width := max(20, f.Width)
	mode := f.Mode
	if mode == "" {
		mode = "READ ONLY"
	}
	title := "dots kube · " + Clean(f.Title)
	tone := Green
	if mode == "ACTION" {
		tone = Yellow
	}
	if f.Environment == "prod" {
		tone = Red
	}
	right := Clip(strings.ToUpper(Clean(f.Environment))+"  "+mode, width-4)
	left := Clip(title, max(1, width-Width(right)-2))
	space := max(1, width-Width(left)-Width(right))
	context, namespace := Clean(f.Context), Clean(f.Namespace)
	if context == "" {
		context = "—"
	}
	if namespace == "" {
		namespace = "—"
	}
	if mode == "LOCAL CONFIG" && f.Context == "" && f.Namespace == "" {
		return Paint(left, Accent+Bold, f.Color) + strings.Repeat(" ", space) + Paint(right, tone+Bold, f.Color) + "\n" + Paint("Локальная настройка · без запросов к кластеру", Dim, f.Color) + "\n" + Paint(strings.Repeat("─", width), Border, f.Color) + "\n"
	}
	return Paint(left, Accent+Bold, f.Color) + strings.Repeat(" ", space) + Paint(right, tone+Bold, f.Color) + "\n" +
		Paint(Clip("context: "+context+"   namespace: "+namespace, width), Dim, f.Color) + "\n" +
		Paint(strings.Repeat("─", width), Border, f.Color) + "\n"
}
func (f Frame) Render() string {
	width := max(20, f.Width)
	out := f.Header()
	body := strings.TrimSuffix(f.Body, "\n")
	lines := strings.Split(body, "\n")
	budget := len(lines)
	if f.Height > 0 {
		budget = max(1, f.Height-5)
	}
	if len(lines) > budget {
		lines = append(lines[:max(0, budget-1)], Paint("… More details are available in a wider terminal", Dim, f.Color))
	}
	for _, line := range lines {
		if !f.Color {
			line = Plain(line)
		}
		out += Clip(line, width) + "\n"
	}
	if f.Footer != "" {
		out += Paint(strings.Repeat("─", width), Border, f.Color) + "\n" + Paint(Clip(Clean(f.Footer), width), Dim, f.Color) + "\n"
	}
	return out
}
func Draw(file *os.File, frame string) {
	if term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(file, "\x1b[H", strings.ReplaceAll(frame, "\n", "\r\n"), "\x1b[J")
	} else {
		fmt.Fprint(file, Plain(frame))
	}
}
