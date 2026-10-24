package anime

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"dots/native/internal/tui"
)

type options struct {
	quality                  int
	query, start, mpvOptions string
}

func parseOptions(args []string) (options, error) {
	o := options{}
	flags := flag.NewFlagSet("anime", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var ongoing, history bool
	source := "anilibria"
	flags.IntVar(&o.quality, "quality", 1080, "preferred maximum quality; 0 = automatic")
	flags.IntVar(&o.quality, "q", 1080, "preferred maximum quality")
	flags.StringVar(&o.query, "search", "", "anime title")
	flags.StringVar(&o.mpvOptions, "mpv-opts", "", "extra mpv options")
	flags.StringVar(&o.mpvOptions, "mo", "", "extra mpv options")
	flags.StringVar(&source, "source", source, "anilibria")
	flags.StringVar(&source, "s", source, "anilibria")
	flags.BoolVar(&ongoing, "ongoing", false, "open ongoing titles")
	flags.BoolVar(&history, "history", false, "open history")
	if err := flags.Parse(args); err != nil {
		return o, err
	}
	if flags.NArg() != 0 {
		return o, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if o.quality < 0 {
		return o, fmt.Errorf("quality must be non-negative")
	}
	if source != "anilibria" {
		return o, fmt.Errorf("this TUI supports anilibria; use anicli --legacy for other sources")
	}
	modes := 0
	if o.query != "" {
		o.start = "search"
		modes++
	}
	if ongoing {
		o.start = "ongoing"
		modes++
	}
	if history {
		o.start = "history"
		modes++
	}
	if modes > 1 {
		return o, fmt.Errorf("choose only one of --search, --ongoing and --history")
	}
	return o, nil
}

const help = `dots anime · русский каталог в общей Ayu TUI

Usage: anicli [--search TITLE | --ongoing | --history] [-q QUALITY]
  --quality, -q N       Предпочтительное качество (1080 по умолчанию, 0 = авто)
  --mpv-opts STRING     Дополнительные параметры mpv без запуска shell
  --source, -s         anilibria (AniLiberty)
  -h, --help           Справка

Enter: открыть / смотреть · Esc: назад · Ctrl+O: качество на экране серий
Для оригинального клиента: anicli --legacy [аргументы anicli-ru cli]
`

func Run(root string, args []string) int {
	opts, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(help)
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "anime:", tui.CleanText(err.Error()))
		return 2
	}
	terminal, err := tui.Open()
	if err == nil {
		terminal.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "anime:", err)
		return 1
	}
	backend, err := newBridge(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "anime:", tui.CleanText(err.Error()))
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	app := application{ui: terminalUI{}, backend: backend, options: opts, play: playVideo}
	if err = app.run(ctx); err != nil && !canceled(err) {
		fmt.Fprintln(os.Stderr, "anime:", tui.CleanText(err.Error()))
		return 1
	}
	return 0
}

type userInterface interface {
	Choose(context.Context, tui.Frame, []string) (int, error)
	Input(context.Context, tui.Frame, string) (string, error)
	Wait(context.Context, tui.Frame, func(context.Context) error) error
	Notice(context.Context, string, string)
}
type application struct {
	ui      userInterface
	backend service
	options options
	play    func(context.Context, Video, []string, string, func() error) error
}

func frame(title, subtitle string) tui.Frame {
	return tui.Frame{Brand: "dots anime", Title: title, Subtitle: subtitle, Targets: []string{"AniLiberty"}, Mode: "КАТАЛОГ", Tone: tui.Green, Footer: "↑↓ выбор · Enter открыть · печатайте для фильтра · Esc назад"}
}
func canceled(err error) bool {
	return errors.Is(err, tui.ErrCanceled) || errors.Is(err, context.Canceled)
}

func (a *application) run(ctx context.Context) error {
	mode := a.options.start
	query := a.options.query
	for ctx.Err() == nil {
		if mode == "" {
			i, err := a.ui.Choose(ctx, frame("Аниме на русском", "Поиск, текущие релизы и ваша история. Воспроизведение в mpv."), []string{"Поиск аниме", "Онгоинги", "История просмотра", "Выход"})
			if canceled(err) || i == 3 {
				return nil
			}
			if err != nil {
				return err
			}
			mode = []string{"search", "ongoing", "history"}[i]
		}
		err := a.catalog(ctx, mode, query)
		if err != nil && !canceled(err) {
			a.ui.Notice(ctx, "Не удалось загрузить каталог", err.Error())
		}
		mode, query = "", ""
	}
	return ctx.Err()
}

func (a *application) catalog(ctx context.Context, mode, query string) error {
	title := map[string]string{"search": "Результаты поиска", "ongoing": "Онгоинги", "history": "История просмотра"}[mode]
	if mode == "search" && query == "" {
		f := frame("Поиск аниме", "Введите русское или оригинальное название")
		f.Prompt = "Название"
		var err error
		query, err = a.ui.Input(ctx, f, "")
		if err != nil {
			return err
		}
		if strings.TrimSpace(query) == "" {
			return nil
		}
	}
	var response struct {
		Items []Item `json:"items"`
	}
	if err := a.ui.Wait(ctx, frame(title, "Загружаю каталог…"), func(ctx context.Context) error {
		return a.backend.Call(ctx, mode, map[string]any{"query": query}, &response)
	}); err != nil {
		return err
	}
	if len(response.Items) == 0 {
		text := "Ничего не найдено. Попробуйте другое название."
		if mode == "history" {
			text = "Здесь появятся последние запуски серий через anicli. История AniLiberty общая с оригинальным клиентом."
		}
		a.ui.Notice(ctx, title, text)
		return nil
	}
	selected := 0
	for ctx.Err() == nil {
		rows := make([]string, len(response.Items))
		for i, item := range response.Items {
			rows[i] = item.Title + "\t" + metadata(item)
			if item.LastEpisode != "" {
				rows[i] += " · запуск: " + item.LastEpisode
			}
		}
		f := frame(title, query)
		if mode == "history" {
			f.Subtitle = "Последние запущенные серии, не отметка полного просмотра"
		}
		f.InitialIndex = selected
		i, err := a.ui.Choose(ctx, f, rows)
		if err != nil {
			return err
		}
		selected = i
		if err = a.episodes(ctx, &response.Items[i]); err != nil && !canceled(err) {
			a.ui.Notice(ctx, "Не удалось открыть серии", err.Error())
		}
	}
	return ctx.Err()
}

func metadata(item Item) string {
	parts := []string{}
	if item.Year > 0 {
		parts = append(parts, fmt.Sprint(item.Year))
	}
	if item.Kind != "" {
		parts = append(parts, item.Kind)
	}
	if item.Count > 0 {
		parts = append(parts, fmt.Sprintf("%d серий", item.Count))
	}
	return strings.Join(parts, " · ")
}

func qualityLabel(q int) string {
	if q == 0 {
		return "авто"
	}
	return fmt.Sprintf("%dp", q)
}
func (a *application) quality(ctx context.Context) error {
	qualities := []int{0, 1080, 720, 480}
	f := frame("Качество видео", "При отсутствии выбранного качества используется ближайшее доступное ниже")
	rows := []string{"Авто · максимальное доступное", "1080p", "720p", "480p"}
	for i, q := range qualities {
		if q == a.options.quality {
			f.InitialIndex = i
		}
	}
	i, err := a.ui.Choose(ctx, f, rows)
	if err == nil {
		a.options.quality = qualities[i]
	}
	return err
}

func (a *application) episodes(ctx context.Context, item *Item) error {
	var response struct {
		Episodes []Episode `json:"episodes"`
	}
	if err := a.ui.Wait(ctx, frame(item.Title, "Загружаю серии…"), func(ctx context.Context) error {
		return a.backend.Call(ctx, "episodes", map[string]any{"item": item}, &response)
	}); err != nil {
		return err
	}
	if len(response.Episodes) == 0 {
		a.ui.Notice(ctx, item.Title, "Серии пока недоступны.")
		return nil
	}
	selected := 0
	for i, episode := range response.Episodes {
		if episode.label() == item.LastEpisode {
			selected = i
		}
	}
	for ctx.Err() == nil {
		rows := make([]string, len(response.Episodes))
		for i, episode := range response.Episodes {
			rows[i] = episode.label()
			if episode.label() == item.LastEpisode {
				rows[i] += " · последний запуск"
			}
		}
		f := frame(item.Title, metadata(*item))
		f.Targets = append(f.Targets, qualityLabel(a.options.quality))
		f.Intro = item.Description
		f.Action = "Качество"
		f.Footer = "Enter смотреть · Ctrl+O качество · PgUp/PgDn описание · Esc назад"
		f.InitialIndex = selected
		i, err := a.ui.Choose(ctx, f, rows)
		if errors.Is(err, tui.ErrAction) {
			if err = a.quality(ctx); err != nil && !canceled(err) {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		selected = i
		if err = a.watch(ctx, item, response.Episodes[i]); err != nil && !canceled(err) {
			a.ui.Notice(ctx, "Не удалось запустить серию", err.Error())
		}
	}
	return ctx.Err()
}

func (a *application) watch(ctx context.Context, item *Item, episode Episode) error {
	var response struct {
		Videos []Video  `json:"videos"`
		Args   []string `json:"args"`
	}
	if err := a.ui.Wait(ctx, frame(item.Title, "Получаю видеопоток…"), func(ctx context.Context) error {
		return a.backend.Call(ctx, "resolve", map[string]any{"episode": episode, "mpv_opts": a.options.mpvOptions}, &response)
	}); err != nil {
		return err
	}
	video, err := chooseVideo(response.Videos, a.options.quality)
	if err != nil {
		return err
	}
	f := frame(item.Title, "Окно mpv открыто. Закройте плеер, чтобы вернуться к сериям.")
	f.Mode = "MPV"
	f.Targets = append(f.Targets, qualityLabel(video.Quality))
	f.Body = episode.label()
	return a.ui.Wait(ctx, f, func(ctx context.Context) error {
		return a.play(ctx, video, response.Args, item.Title+" · "+episode.label(), func() error {
			if err := a.backend.Call(ctx, "remember", map[string]any{"item": item, "episode": episode}, nil); err != nil {
				return err
			}
			item.LastEpisode = episode.label()
			return nil
		})
	})
}
