package anime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"dots/native/internal/tui"
)

type scriptedUI struct {
	t       *testing.T
	choices []int
	inputs  []string
	frames  []tui.Frame
	notices []string
}

func (u *scriptedUI) Choose(_ context.Context, f tui.Frame, _ []string) (int, error) {
	u.frames = append(u.frames, f)
	if len(u.choices) == 0 {
		u.t.Fatal("unexpected screen", f.Title)
		return 0, tui.ErrCanceled
	}
	i := u.choices[0]
	u.choices = u.choices[1:]
	if i == -1 {
		return 0, tui.ErrCanceled
	}
	if i == -2 {
		return 0, tui.ErrAction
	}
	return i, nil
}
func (u *scriptedUI) Input(_ context.Context, _ tui.Frame, _ string) (string, error) {
	if len(u.inputs) == 0 {
		u.t.Fatal("unexpected input")
		return "", tui.ErrCanceled
	}
	v := u.inputs[0]
	u.inputs = u.inputs[1:]
	return v, nil
}
func (u *scriptedUI) Wait(ctx context.Context, _ tui.Frame, task func(context.Context) error) error {
	return task(ctx)
}
func (u *scriptedUI) Notice(_ context.Context, title, text string) {
	u.notices = append(u.notices, title+": "+text)
}

type catalogService struct {
	calls      []string
	remembered Episode
	fail       bool
	empty      bool
}

func (s *catalogService) Call(_ context.Context, op string, input, output any) error {
	s.calls = append(s.calls, op)
	if s.fail {
		return errors.New("catalog unavailable")
	}
	var result any
	switch op {
	case "search", "ongoing", "history":
		items := []Item{{Title: "Фрирен", Year: 2023, Description: "Путь мага", LastEpisode: "5 Фантом", Data: json.RawMessage(`{"id":9542}`)}}
		if s.empty {
			items = nil
		}
		result = map[string]any{"items": items}
	case "episodes":
		result = map[string]any{"episodes": []Episode{{Ordinal: 1, Title: "Начало"}, {Ordinal: 5, Title: "Фантом"}}}
	case "resolve":
		result = map[string]any{"videos": []Video{{Quality: 480, URL: "https://example.com/480.m3u8"}, {Quality: 720, URL: "https://example.com/720.m3u8"}, {Quality: 1080, URL: "https://example.com/1080.m3u8"}}, "args": []string{"--vo=null"}}
	case "remember":
		s.remembered = input.(map[string]any)["episode"].(Episode)
		return nil
	default:
		return errors.New("unexpected op " + op)
	}
	data, _ := json.Marshal(result)
	return json.Unmarshal(data, output)
}

func TestSharedTUISearchPlaybackAndBack(t *testing.T) {
	ui := &scriptedUI{t: t, choices: []int{0, 0, 1, -1, -1, 3}, inputs: []string{"Фрирен"}}
	service := &catalogService{}
	played := 0
	app := application{ui: ui, backend: service, options: options{quality: 720}, play: func(ctx context.Context, v Video, args []string, title string, onStart func() error) error {
		played++
		if v.Quality != 720 || !strings.Contains(title, "Фантом") || len(args) != 1 {
			t.Fatalf("bad playback: %#v %v %s", v, args, title)
		}
		return onStart()
	}}
	if err := app.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if played != 1 || service.remembered.Ordinal != 5 || strings.Join(service.calls, ",") != "search,episodes,resolve,remember" {
		t.Fatalf("workflow: played=%d, service=%#v", played, service)
	}
	if len(ui.notices) != 0 || len(ui.choices) != 0 {
		t.Fatalf("notices %v, choices %v", ui.notices, ui.choices)
	}
	for _, frame := range ui.frames {
		if frame.Brand != "dots anime" {
			t.Fatal("not shared frame", frame)
		}
	}
	if ui.frames[3].InitialIndex != 1 {
		t.Fatal("lost selected episode", ui.frames[3])
	}
}

func TestHistoryOpensLastEpisodeWithoutMarkingAnythingWatched(t *testing.T) {
	ui := &scriptedUI{t: t, choices: []int{0, -1, -1, 3}}
	service := &catalogService{}
	app := application{ui: ui, backend: service, options: options{quality: 1080, start: "history"}}
	if err := app.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(service.calls, ",") != "history,episodes" || ui.frames[1].InitialIndex != 1 {
		t.Fatalf("history navigation: %v %#v", service.calls, ui.frames)
	}
}

func TestQualityCanBeChangedOnEpisodeScreen(t *testing.T) {
	ui := &scriptedUI{t: t, choices: []int{0, -2, 2, 0, -1, -1, 3}}
	service := &catalogService{}
	app := application{ui: ui, backend: service, options: options{quality: 1080, start: "ongoing"}, play: func(_ context.Context, v Video, _ []string, _ string, onStart func() error) error {
		if v.Quality != 720 {
			t.Fatal("quality not updated", v)
		}
		return onStart()
	}}
	if err := app.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if app.options.quality != 720 {
		t.Fatal(app.options)
	}
}

func TestEmptyAndFailedCatalogStayNavigable(t *testing.T) {
	for _, fail := range []bool{false, true} {
		ui := &scriptedUI{t: t, choices: []int{3}}
		app := application{ui: ui, backend: &catalogService{empty: true, fail: fail}, options: options{start: "ongoing"}}
		if err := app.run(context.Background()); err != nil || len(ui.notices) != 1 {
			t.Fatalf("recoverable catalog: %v, %v", err, ui.notices)
		}
	}
}

func TestOptionsValidateBeforeRuntime(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"-q", "-1"}, {"--search", "Фрирен", "--ongoing"}, {"--source", "other"}, {"unexpected"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatal("accepted invalid args", args)
		}
	}
	o, err := parseOptions([]string{"--search", "Провожающая", "-q", "720", "--mpv-opts", "--vo=null --ao=null"})
	if err != nil || o.query != "Провожающая" || o.quality != 720 || o.start != "search" || o.mpvOptions != "--vo=null --ao=null" {
		t.Fatalf("options %#v, %v", o, err)
	}
}
