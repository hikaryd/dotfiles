# Рабочее окружение

Dotfiles для macOS, стиль **Ayu Dark**. Ghostty, Nushell/zsh, Herdr/tmux,
AeroSpace/Rift, Neovim, Yazi, LazyGit, Codex/Claude/pi.
Пакеты — [`Brewfile`](Brewfile), настройки — [`config/`](config/).

## Установка

Нужны Xcode Command Line Tools, Homebrew и Go 1.26+. Репозиторий — `~/dots`:

```sh
./install
./scripts/build-go-tools.sh   # пересобрать собственные утилиты после изменений
```

Собственные утилиты — Go-модуль `tools/dots`, общий бинарник `dots-tools`.
Алиасы: `dots`, `dots-nu`, `ai_helper`, `dots-vpn`, `dots-task-titles`.
`build/bin/` содержит локальные ссылки и не попадает в Git.
Shell-файлы связывают установку и приложения; Python нужен стороннему Dotbot,
не runtime собственных команд.

## Команды и проверки

```sh
dots                         # меню с поиском и цветами Ayu
dots help
dots menu --preview --plain   # посмотреть меню без запуска действий
dots private init
dots private check
dots-tools palette --help
dots-vpn --help
make test                    # Go race/vet, Node, проверки запуска тестов
```

В меню: ↑/↓ — выбор, Enter — открыть, Esc — отмена. Справка сгруппирована;
`NO_COLOR` отключает цвета, `--json` остаётся чистым выводом для скриптов.
Отдельно: `make test-go`, `make test-node`, `make test-runner`.
Секреты, пользовательские настройки и рабочие проекты остаются вне Git:
[приватные файлы](docs/PRIVATE.md). [VPN](vpn/README.md) настраивается отдельно.
