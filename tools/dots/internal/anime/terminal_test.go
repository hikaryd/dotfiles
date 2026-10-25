package anime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"dots/native/tests/testutil/testpty"
)

type slowCatalog struct{}

func (slowCatalog) Call(ctx context.Context, _ string, _, _ any) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestAnimeTerminalHelper(t *testing.T) {
	mode := os.Getenv("DOTS_ANIME_TERMINAL_TEST")
	if mode == "" {
		return
	}
	app := application{ui: terminalUI{}, backend: &catalogService{}, options: options{quality: 1080}, play: func(ctx context.Context, _ Video, _ []string, _ string, started func() error) error {
		if err := started(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
			return nil
		}
	}}
	if mode == "cancel" {
		app.backend = slowCatalog{}
		app.options.start = "ongoing"
	}
	if err := app.run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
func animeTerminal(t *testing.T, mode string) *testpty.CLI {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAnimeTerminalHelper$")
	cmd.Env = append(os.Environ(), "DOTS_ANIME_TERMINAL_TEST="+mode, "TERM=xterm-256color")
	return testpty.Start(t, cmd)
}
func TestAnimeRealTerminalNavigation(t *testing.T) {
	p := animeTerminal(t, "navigation")
	p.Screen("Аниме на русском")
	p.Send("\r")
	p.Screen("Введите русское")
	p.Send("\x1b[200~Фрирен\x1b[201~\r")
	p.Screen("2023")
	offset := len(p.Text())
	p.Send("\r")
	p.After(offset, "Путь мага")
	offset = len(p.Text())
	p.Send("\x0f")
	p.After(offset, "Качество видео")
	offset = len(p.Text())
	p.Send("\x1b[B\r")
	p.After(offset, "AniLiberty  720p")
	offset = len(p.Text())
	p.Send("\r")
	p.After(offset, "MPV")
	p.After(offset, "последний запуск")
	offset = len(p.Text())
	p.Send("\x1b")
	p.After(offset, "Результаты поиска")
	offset = len(p.Text())
	p.Send("\x1b")
	p.After(offset, "Аниме на русском")
	p.Send("\x1b")
	p.Wait(0)
}
func TestAnimeLoadingCancelAndResizeRestoreTerminal(t *testing.T) {
	p := animeTerminal(t, "cancel")
	p.Screen("Загружаю каталог")
	p.Resize(44, 16)
	p.Send("\x1b")
	p.Screen("История просмотра")
	p.Send("\x1b")
	p.Wait(0)
}
