package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dots/native/internal/tui"
)

const menuKeys = "↑/↓ / Ctrl+N/P выбор · Enter открыть · Esc отмена"

type menuAction struct {
	key, group, description string
}

// One source for the preview and the picker's exact-match whitelist.
var menuActions = []menuAction{
	{"sessions", "Работа", "Рабочие пространства / проект по имени"},
	{"projects", "Работа", "Проекты и worktree"},
	{"git", "Работа", "LazyGit здесь"},
	{"refs", "Инструменты", "Ссылки / ID / file:line"},
	{"clip", "Инструменты", "JSON / Base64 / JWT / время"},
	{"status", "Проверки", "Git / процессы / порты"},
	{"ports", "Проверки", "TCP listeners · без kill"},
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

func desktopFrame(title string) tui.Frame {
	return tui.Frame{Brand: "dots", Title: title, Mode: "LOCAL", Tone: tui.Green}
}

func menuRun(root string, args []string) int {
	o, err := parse(args, "--preview --plain", "")
	if err != nil || len(o.pos) != 0 || o.flag("--plain") && !o.flag("--preview") {
		return usageError("menu")
	}
	if !o.flag("--preview") {
		return menu(root)
	}
	frame := desktopFrame(menuTitle("preview"))
	frame.Width, _ = tui.Size(os.Stdout)
	frame.Color = !o.flag("--plain") && tui.ColorEnabled(os.Stdout)
	var body strings.Builder
	body.WriteString("Поиск › команда или описание\n\n")
	group := ""
	for i, choice := range menuChoices() {
		if group != menuActions[i].group {
			group = menuActions[i].group
			body.WriteString("\n" + group + "\n")
		}
		body.WriteString("  " + choice[0] + " · " + choice[1] + "\n")
	}
	frame.Body, frame.Footer = body.String(), menuKeys+" · Предпросмотр · действия не выполняются"
	fmt.Print(frame.Render())
	return 0
}
func printMenuHelp() {
	frame := desktopFrame("Меню")
	frame.Width, _ = tui.Size(os.Stdout)
	frame.Color = tui.ColorEnabled(os.Stdout)
	frame.Body = "usage: dots menu [--preview [--plain]]\n\nПоиск по команде или описанию; " + menuKeys + "\nCtrl+L — очистить поиск\n\ndots menu --preview         показать меню без запуска действий\ndots menu --preview --plain обычный текст без цвета"
	fmt.Print(frame.Render())
}
func printHelp() {
	frame := desktopFrame("desktop")
	frame.Width, _ = tui.Size(os.Stdout)
	frame.Color, frame.Body = tui.ColorEnabled(os.Stdout), helpText
	fmt.Print(frame.Render())
}

const helpText = `
Работа
  dots menu                 поиск действий
  dots sessions open        Живые сессии; поиск проекта из ~/Documents/dev
  dots sessions open NAME   Открыть/создать сессию по имени проекта
  dots sessions open --all  Импорт: tmux / сохранённые / история каталогов
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
  Поиск по команде или описанию; ↑/↓ / Ctrl+N/P выбор
  Enter открыть · Esc/Ctrl+C отмена; Ctrl+L — очистить поиск
  zsh Ctrl+O — меню; Ctrl+R — история Atuin
  rmux/tmux/Herdr prefix = Ctrl+A: O меню, P проекты, T сессии
  F refs, Y clipboard, V status

Приватность
  Presentation скрывает контекст меню / prompt / status.
  Содержимое clipboard и scrollback остаётся видимым.
  Существующие проекты и сессии не перезапускаются.`
