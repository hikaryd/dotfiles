# Рабочее окружение

Dotfiles для macOS. Общий стиль — **Ayu Dark**, палитра в [`palette/ayu.toml`](palette/ayu.toml).

## Программы

| Задача | Программы |
|---|---|
| Терминал | Ghostty |
| Оболочка | Nushell, zsh, Starship, Atuin |
| Мультиплексоры | Herdr, tmux, sesh |
| Окна | AeroSpace, Rift |
| Редакторы | Neovim |
| Файлы и поиск | Yazi, nls, eza, bat, ripgrep, fd, fzf, zoxide |
| Git | Git, LazyGit, GitHub CLI |
| AI | Codex, Claude Code, [Pi Agent](config/pi/README.md), CodexBar |
| Разработка | Node.js, Python, Go, Rust; языковые серверы, линтеры и форматтеры |
| Браузер | Waterfox |
| Приложения | Telegram, Spotify, 1Password, RustDesk |
| macOS-утилиты | Vorssaint, Karabiner-Elements, Mos, Snapzy, TinyCast, Caffeine, Look |
| Прокси | Xray |

Полный список пакетов — в [`Brewfile`](Brewfile), настройки — в [`config/`](config/).

## Установка

Нужны Xcode Command Line Tools и Homebrew. Репозиторий располагается в `~/dots`:

```sh
./install
```

Только Pi Agent с личными настройками, темой Ayu и статус-баром:

```sh
./install -c steps/pi.yml
```

Секреты и личные адреса не хранятся в Git. [Приватные настройки и перенос](docs/PRIVATE.md).

## Проверки

```sh
make test       # все собственные тесты: Go, Python, Node и test runner
make test-list  # состав наборов без запуска
```

[Правила и отдельные наборы](tests/README.md).
[Аудит скриптов и порядок переноса на Go](docs/SCRIPT-AUDIT.md).
