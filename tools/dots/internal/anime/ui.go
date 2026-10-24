package anime

import (
	"context"
	"fmt"
	"time"

	"dots/native/internal/tui"
)

type terminalUI struct{}

func (terminalUI) Choose(ctx context.Context, f tui.Frame, rows []string) (int, error) {
	indices, err := tui.Select(ctx, f, rows, "", false)
	if err != nil {
		return 0, err
	}
	if len(indices) != 1 {
		return 0, tui.ErrCanceled
	}
	return indices[0], nil
}
func (terminalUI) Input(ctx context.Context, f tui.Frame, value string) (string, error) {
	return tui.Input(ctx, f, value)
}
func (u terminalUI) Notice(ctx context.Context, title, text string) {
	f := frame(title, "")
	f.Intro = tui.CleanText(text)
	f.Tone = tui.Yellow
	f.Footer = "Enter или Esc назад"
	_, _ = u.Choose(ctx, f, []string{"Назад"})
}

// Wait keeps both loading and playback cancellable, and restores the same shared
// terminal lifecycle as the picker even when a subprocess or network call fails.
func (terminalUI) Wait(ctx context.Context, f tui.Frame, task func(context.Context) error) error {
	terminal, err := tui.Open()
	if err != nil {
		return err
	}
	defer terminal.Close()
	restore, err := tui.Raw(terminal)
	if err != nil {
		return err
	}
	defer restore()
	closeScreen := tui.Screen(terminal)
	defer closeScreen()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- task(ctx) }()
	keys := tui.Keys(ctx, terminal)
	painter := tui.NewPainter(terminal)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	spinner := []string{"◐", "◓", "◑", "◒"}
	step := 0
	body := f.Body
	stopping := false
	done := ctx.Done()
	for {
		f.Width, f.Height = tui.Size(terminal)
		f.Color = tui.ColorEnabled(terminal)
		f.Footer = "Esc остановить и вернуться · Ctrl+C отменить"
		if stopping {
			f.Body = "Останавливаю…"
		} else {
			f.Body = fmt.Sprintf("%s  %s", spinner[step%len(spinner)], body)
		}
		painter.Draw(f)
		select {
		case err := <-result:
			if stopping {
				return tui.ErrCanceled
			}
			return err
		case <-done:
			stopping = true
			done = nil
		case key, ok := <-keys:
			if !ok {
				keys = nil
				continue
			}
			if key.Name == "esc" || key.Name == "ctrl-c" || key.Name == "ctrl-d" {
				stopping = true
				cancel()
				done = nil
			}
		case <-ticker.C:
			step++
		}
	}
}
