package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ayuAccent  = "#73d0ff"
	ayuText    = "#bfbdb6"
	ayuMuted   = "#65758b"
	ayuBorder  = "#273747"
	ayuSuccess = "#95e6cb"
	ayuWarning = "#e6b450"
	menuKeys   = "↑/↓ выбор · Enter открыть · Esc отмена"
)

type menuAction struct {
	key, group, description string
}

// One source for the preview and the picker's exact-match whitelist.
var menuActions = []menuAction{
	{"sessions", "Работа", "Herdr / tmux / каталоги"},
	{"projects", "Работа", "Проекты и worktree"},
	{"git", "Работа", "LazyGit здесь"},
	{"refs", "Инструменты", "Ссылки / ID / file:line"},
	{"clip", "Инструменты", "JSON / Base64 / JWT / время"},
	{"status", "Проверки", "Git / процессы / порты"},
	{"ports", "Проверки", "TCP listeners · без kill"},
	{"kube", "Проверки", "Kubernetes · read-only"},
	{"doctor", "Проверки", "Диагностика сетапа"},
	{"style", "Настройка", "Glass / Focus / Presentation"},
	{"config", "Настройка", "Открыть конфиг"},
	{"help", "Настройка", "Команды и примеры"},
}

func menuChoices() [][2]string {
	choices := make([][2]string, 0, len(menuActions))
	for _, a := range menuActions {
		choices = append(choices, [2]string{a.key, a.group + " / " + a.description})
	}
	return choices
}

func menuTitle(context string) string {
	if context == "" {
		return "Меню"
	}
	return "Меню · " + context
}

func menuContext() string {
	style, err := loadStyle()
	// If privacy state cannot be read, omit context rather than expose it.
	if err != nil || style == "presentation" {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil || cwd == string(filepath.Separator) {
		return ""
	}
	return truncate(printable(filepath.Base(cwd)), 24)
}

func colorEnabled(output *os.File) bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	return !noColor && os.Getenv("TERM") != "dumb" && isTerminal(output)
}

func ayu(s, hex string, enabled bool) string {
	if !enabled {
		return s
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, s)
}

func selectorHeader(title string) string {
	return "dots · " + title + "\n" + menuKeys
}

func menuRun(root string, args []string) int {
	o, err := parse(args, "--preview --plain", "")
	if err != nil || len(o.pos) != 0 || o.flag("--plain") && !o.flag("--preview") {
		return usageError("menu")
	}
	if !o.flag("--preview") {
		return menu(root)
	}
	// A static context deliberately avoids reading private profile or live data.
	color := !o.flag("--plain") && colorEnabled(os.Stdout)
	fmt.Println(ayu(selectorHeader(menuTitle("preview")), ayuAccent, color))
	fmt.Println(ayu("Поиск > команда или описание", ayuMuted, color))
	group := ""
	for i, c := range menuChoices() {
		if group != menuActions[i].group {
			fmt.Println()
			group = menuActions[i].group
		}
		fmt.Println("  " + ayu(c[0], ayuAccent, color) + ayu(" · "+c[1], ayuText, color))
	}
	fmt.Println()
	fmt.Println(ayu("Предпросмотр · действия не выполняются", ayuSuccess, color))
	return 0
}

// Selectors own the terminal UI; their stdout remains an undecorated selection.
// Normalize choose to filter so clipboard and refs also start with search.
func selectorArgs(gum, fzf []string, color bool) ([]string, []string) {
	gum = append([]string{}, gum...)
	fzf = append([]string{}, fzf...)
	title := "Поиск"
	for i := 0; i+1 < len(gum); i++ {
		if has(gum[i], "--header", "--placeholder") {
			title = gum[i+1]
		}
	}
	if len(gum) > 0 && gum[0] == "choose" {
		gum = []string{"filter", "--limit", "1", "--no-sort"}
	}
	// Replace only display flags; retain caller ordering and selection limits.
	filtered := gum[:0]
	for i := 0; i < len(gum); i++ {
		if has(gum[i], "--header", "--placeholder") && i+1 < len(gum) {
			i++
			continue
		}
		filtered = append(filtered, gum[i])
	}
	gumHeader := "dots · " + title + "\n↑/↓ выбор · Enter открыть · Ctrl+C отмена (Esc ×2)"
	gum = append(filtered, "--header", gumHeader, "--placeholder", "Поиск: команда или описание", "--padding", "0 1")
	colors := []string{ayuAccent, ayuText, ayuSuccess, ayuMuted, ayuAccent, ayuAccent, ayuText}
	for i, flag := range []string{"--indicator.foreground", "--text.foreground", "--match.foreground", "--placeholder.foreground", "--prompt.foreground", "--header.foreground", "--cursor-text.foreground"} {
		value := colors[i]
		if !color {
			value = ""
		}
		gum = append(gum, flag, value)
	}
	fzf = append(fzf, "--layout=reverse", "--border=rounded", "--header", selectorHeader(title), "--bind", "ctrl-n:down,ctrl-p:up,esc:abort")
	palette := "bw"
	if color {
		palette = strings.Join([]string{"fg:" + ayuText, "fg+:" + ayuText, "hl:" + ayuSuccess, "hl+:" + ayuSuccess, "border:" + ayuBorder, "header:" + ayuMuted, "prompt:" + ayuAccent, "pointer:" + ayuAccent, "marker:" + ayuSuccess, "info:" + ayuWarning}, ",")
	}
	return gum, append(fzf, "--color", palette)
}

func printMenuHelp() {
	color := colorEnabled(os.Stdout)
	fmt.Println(ayu("dots · Меню", ayuAccent, color))
	fmt.Println("usage: dots menu [--preview [--plain]]")
	fmt.Println("\nПоиск по команде или описанию; " + menuKeys)
	fmt.Println("Ctrl+N/P — следующий / предыдущий пункт. Gum fallback: Ctrl+C / Esc ×2 — отмена.")
	fmt.Println("\ndots menu --preview         показать меню без запуска действий")
	fmt.Println("dots menu --preview --plain обычный текст без цвета")
}

func printHelp() {
	color := colorEnabled(os.Stdout)
	fmt.Println(ayu("dots · desktop", ayuAccent, color))
	for _, line := range strings.Split(helpText, "\n") {
		tone := ayuText
		if line != "" && !strings.HasPrefix(line, " ") {
			tone = ayuAccent
		}
		fmt.Println(ayu(line, tone, color))
	}
}

const helpText = `
Работа
  dots menu                 поиск действий
  dots sessions open        Herdr / tmux / каталоги
  dots projects open        проекты и worktree
  dots projects list        inventory; --json для скриптов
  dots projects refresh     обновить приватный inventory

Инструменты
  dots refs                 ссылки / ID / file:line из панели
                            --list или --json без открытия
  dots clip                 JSON / Base64 / JWT / time / redact
                            запись с подтверждением

Проверки
  dots status [--all]        Git / процессы / порты; --json
                            --interactive: r / o / q
  dots ports                TCP listeners · без kill
  dots kube                 Kubernetes · read-only
  dots doctor [--strict]    диагностика; --json

Настройка
  dots style glass|focus|presentation
  dots style status         текущий профиль
  dots private init|check    приватные файлы без вывода секретов
  dots tmux-style --current применить текущий профиль tmux
  dots ghostty-reload        перечитать конфиг Ghostty
  dots herdr-status         краткий статус Herdr
  dots notify --test        тест уведомления
  dots nu [args...]          запустить Nushell

Примеры
  dots menu --preview --plain
  dots clip json --stdout
  dots projects list --json
  dots refs --stdin --json
  dots <команда> --help      все аргументы команды

Клавиши
  Поиск по команде или описанию; ↑/↓ выбор
  Enter открыть · Esc отмена; Ctrl+N/P — выбор
  Gum fallback: Ctrl+C / Esc ×2 — отмена
  zsh Ctrl+O — меню; Ctrl+R — история Atuin
  tmux/Herdr prefix = Ctrl+A: O меню, P проекты, T сессии
  F refs, Y clipboard, V status

Приватность
  Presentation скрывает контекст меню / prompt / status.
  Содержимое clipboard и scrollback остаётся видимым.
  Существующие проекты и сессии не перезапускаются.`
