# Native Go UI: реализация и повторное использование

Это описание **текущей реализации**, а не новой библиотеки. Эталон — Kubernetes
CLI в `tools/dots`; UI работает без gum, zsh, Bubble Tea и Lip Gloss, на macOS/Linux.
В одном Go-модуле этот же код можно использовать в других тулах.

## Module, Interface, Implementation, Seam

- **Module**: `tools/dots/internal/kube/tui`. Он владеет терминалом,
  клавиатурой, Ayu-стилем, раскладкой, селекторами и incremental rendering.
- **Interface**: `Frame`, `Select`, `Input`, `Editor`, `Painter`, `Keys`,
  `Open`, `Raw`, `Screen` и небольшие функции форматирования. Помимо типов,
  контракт включает отмену, восстановление терминала и режим без TTY.
- **Implementation**: ANSI escape sequences, platform-specific termios,
  декодирование ввода и сравнение строк предыдущего/следующего кадра.
- **Seam**: вызывающий код передаёт готовые строки и состояние; получает
  индексы выбранных строк, текст или событие клавиатуры. Kubernetes-запросы,
  файловые операции, бизнес-правила и подтверждение записи остаются у вызывающего.

## Карта исходников

Пути ниже — относительно `tools/dots/internal/kube/`.

| Файл | Ответственность |
|---|---|
| `tui/style.go` | Ayu palette, terminal-cell width, `Frame.Header/Render`, очистка текста |
| `tui/painter.go` | Diff строк, resize, synchronized write, отсутствие bottom-row newline |
| `tui/terminal.go` | `/dev/tty`, raw/alternate screen, клавиатура и bracketed paste |
| `tui/read_{darwin,linux}.go` | Ограниченное ожидание ввода через `VMIN=0`, `VTIME=1` |
| `tui/picker.go` | `Select`, `Input`, `Editor` и их UI-состояние |
| `ui.go` | Kubernetes-Adapter: domain header, resource selection, запасной gum |
| `wizard.go`, `namespace_discovery.go` | Мастер: файлы → context → namespace → подтверждение |
| `logwatch/` | Поток логов, bounded history, pause, search, details |
| `deploywatch/`, `podanalyze/` | Refresh/polling и domain-specific dashboard presentation |

Зависимости UI: `golang.org/x/term`, `golang.org/x/sys`,
`github.com/mattn/go-runewidth`. `github.com/creack/pty` нужен тестам, не runtime.

## Стиль и компоновка

`Frame` описывает экран: `Title`, `Environment`, `Context`, `Namespace`, `Mode`,
`Intro`, `Prompt`, `Action`, `Body`, `Footer`, `Width`, `Height`, `Color`.
`InitialIndex` — индекс **исходной строки**, выделяемой при открытии `Select`.
После фильтрации возвращаются исходные индексы, а не позиции в видимом списке.

Шапка сейчас содержит бренд **dots kube**, окружение, context, namespace и режим:
`READ ONLY`, `ACTION`, `LOCAL CONFIG`. Prod выделяется красным; action — жёлтым.
Общая палитра задаётся константами: cyan accent, mint success, amber warning,
red error, purple detail, muted text/borders. Не вводить локальные варианты темы.

`Frame.Render()` — чистый рендер текста. `Width/Clip/Pad` используют ширину
терминальных ячеек, не число байтов. Длинный body ограничивается высотой экрана;
при `Height=0` ограничение высоты не применяется. `Select` отдельно переносит и
прокручивает длинное объяснение на узком экране. Это **не** универсальный scrolling
layout: длинный `Input.Intro` и произвольный dashboard требуют своего layout.

Для внешних имён и сообщений использовать `Clean` или `CleanText`, чтобы не
превращать данные в управление терминалом. `CleanText` сохраняет переносы строк.
Цвет включать через `ColorEnabled`, который учитывает TTY, `NO_COLOR` (даже пустой)
и `TERM=dumb`. Machine-readable stdout не должен проходить через `Frame`.

## Единый контракт клавиатуры

**F-клавиши не используются.**

| Экран | Управление |
|---|---|
| Selector | ↑↓, Home/End; ввод — поиск; Ctrl+L — очистить; Enter — выбрать; Esc — назад |
| Multiselect | Space/Tab — отметить, в том числе после поиска; Ctrl+A — все совпадения; Enter — все отмеченные |
| Дополнительное действие | Ctrl+O, только когда задан `Frame.Action` |
| Input | Ввод/backspace, Ctrl+L — очистить, Enter — подтвердить, Esc — назад |
| Multiline editor | Enter — новая строка, Ctrl+D — отправить, Esc/Ctrl+C — отменить |
| Logs | Space/p — pause; `/` — поиск; f — filter; c — clear; m — full/compact; ↑↓/j/k — запись; Enter — pause + details |

Отметки multiselect сохраняются при изменении поиска; Enter может отправить
отмеченное даже при нуле текущих совпадений. Space в multiselect — действие,
не пробел в поиске; составной запрос с пробелами можно вставить.
Текущая отмеченная строка показывает и курсор, и checkbox, а не один курсор.

`Keys` возвращает `Key{Name, Text}`. Bracketed paste возвращается **одним** событием
`paste`: не исполнять его символы как hotkeys. Одиночный Esc отличается от escape
sequence коротким ожиданием продолжения. Zero-byte tty timeout не означает EOF.

Ошибки UI:

- `ErrNoTerminal`: выбрать headless-путь или явно сказать, что нужен терминал;
- `ErrCanceled`: назад/отмена, не ошибка подключения;
- `ErrAction`: Ctrl+O; выполнить дополнительное действие и открыть экран снова;
- ошибка context: завершить операцию, а не повторять запрос бесконечно.

`Select` и `Input` одинаково сигнализируют Ctrl+O через `ErrAction`; отрицательных
«служебных индексов» нет. `Input` при этом возвращает введённый черновик — сохранить
его перед открытием дополнительного диалога, чтобы Esc не потерял ввод.

## Минимальный пример для нового тула

Внутри `tools/dots` импорт пока такой:

```go
import (
    "context"
    "errors"
    "fmt"

    "dots/native/internal/kube/tui"
)

type artifact struct { ID, State string }

func chooseArtifacts(ctx context.Context, items []artifact) ([]artifact, error) {
    if len(items) == 0 {
        return nil, errors.New("no artifacts")
    }
    rows := make([]string, len(items))
    for i, item := range items {
        rows[i] = fmt.Sprintf("%s\t%s", tui.Clean(item.ID), tui.Clean(item.State))
    }
    selected, err := tui.Select(ctx, tui.Frame{
        Title: "Choose artifacts",
        Intro: "Search and mark the artifacts to inspect.",
        Mode: "READ ONLY",
    }, rows, "", true)
    if err != nil {
        return nil, err // caller decides whether cancellation means back or exit
    }
    result := make([]artifact, 0, len(selected))
    for _, index := range selected {
        result = append(result, items[index])
    }
    return result, nil
}
```

Labels только отображаются; реальные ID берутся по индексу. Не парсить обратно
выравненную строку и не строить shell-команду из label. Для subprocess использовать
`exec.CommandContext` с отдельными argv и явно закреплённым окружением.

`Select/Input/Editor` сами открывают `/dev/tty`, включают raw/alternate screen,
читают клавиши и восстанавливают состояние. Не оборачивать их ещё одним raw mode.

## Live dashboard: lifecycle и производительность

Для собственного dashboard использовать `Open → Raw → Screen → Keys/Painter`.
На всех путях выхода: отменить worker context, закрыть alternate screen,
восстановить termios и закрыть **собственный** tty descriptor. Не закрывать stdin.
Регистрировать `defer` в обратном порядке очистки.

Подключать клавиатуру **до первого сетевого запроса**. Иначе Esc/q во время
инициализации не работают. Poll/exec должны принимать тот же отменяемый context
и иметь deadline; отмена должна достигать subprocess, а не только event loop.

`Painter.Draw(Frame)` сравнивает строки кадра, обновляет только изменившиеся,
очищает исчезнувшие строки и учитывает resize. Один `?2026` synchronized write
защищает от half-painted frames. Он не очищает весь экран и не печатает newline
в последней строке: иначе начинается прокрутка/дрожание всей шапки.

Сбор данных и rendering разделены:

- redraw по таймеру, изменению состояния, клавише или resize, не на каждую запись;
- data history ограничивается числом записей и байтами;
- pause замораживает снимок, а ingestion продолжает работать;
- полные записи — default; JSON message summaries включаются только явно;
- адаптивные multi-log колонки оставляют имена целыми на широком экране;
- длинные сообщения переносятся, превышение высоты явно отмечается;
- expensive JSON/detail formatting выполняется для видимых записей;
- stdout/stderr и exit status subprocess не заменяются на «красивый текст» в pipe.

Для non-TTY: raw/alternate screen отключены; dashboard — один обычный snapshot,
logs — исходный stream. JSON/сохранённые логи/диагностика имеют отдельные пути.

## Как переносить на остальные Go tools

1. В том же модуле сначала повторно использовать `tui`, а не копировать ANSI-код.
2. Перед массовым переносом вынести Module в `tools/dots/internal/tui` и убрать
   Kubernetes-зависимости из `Frame`: бренд, дополнительные target labels и
   режим/тон передавать явно. Kubernetes-specific mapping оставить в Adapter.
3. Обновить все import paths и сохранить один клавиатурный контракт и Painter.
4. Для **другого Go-модуля/репозитория** нужен отдельный public package/module:
   Go `internal` нельзя импортировать извне. Текущий код ещё не такой пакет.
5. Мигрировать по одному workflow, каждый раз проверяя прежние argv, данные,
   exit status и действия отмены. Цветной preview не заменяет functional test.

## Проверки перед распространением

Публичный CLI — основной test seam; подменяется только внешний kubectl/tool.
Интерактивный код проверяется в настоящем PTY, не fake gum.

- Different sources/context с одинаковыми именами и **разными** ответами.
- Новый screen после действия, не совпадение с текстом старого screen в capture.
- Поиск → отметить → сменить поиск → подтвердить скрытые отметки.
- Ctrl+O → отменить → черновик не потерян; Esc/back не пишет настройки.
- Wide/narrow layout, Unicode, resize, bracketed paste и восстановление termios.
- Сильный поток, pause/details, bounded retention, отсутствие repaint шапки.
- Pipe/JSON/save без ANSI и format changes; failure exit code; stderr отдельно.
- Отмена во время начального fetch и refresh; никаких настоящих мутаций в тестах.

```sh
go -C tools/dots test -race ./internal/kube/... ./cmd/dots-kube -count=1 -timeout=90s
go -C tools/dots vet ./internal/kube/... ./cmd/dots-kube
make test
```
