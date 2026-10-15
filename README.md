# Рабочее окружение

Dotfiles для macOS в стиле Ayu Dark: Ghostty, Nushell/zsh, Herdr/tmux,
AeroSpace/Rift, Neovim, Yazi, LazyGit и Pi.

## Установка

Нужны Xcode Command Line Tools, Homebrew и Go 1.26+. Checkout — `~/dots`.

```sh
./install
make test                          # Go race/vet, Node и shell-контракты
./scripts/build-go-tools.sh         # пересобрать собственные утилиты
```

Пакеты — [Brewfile](Brewfile), ссылки и шаги установки — [steps/](steps/),
настройки — [config/](config/), свои CLI — [tools/dots/](tools/dots/).
Выборочная установка: `./install -c steps/pi.yml`.
Откат: `./uninstall --dry-run`, затем `./uninstall`; без `--packages`
Homebrew-пакеты не удаляются. VPN устанавливается [отдельно](vpn/README.md).

## На каждый день

```sh
dots                               # меню
dots status --interactive          # состояние окружения
dots sessions open                 # сессии и проекты
dots-tools palette --help          # общая тема
```

В меню: ↑↓ / Ctrl+N/P — выбор, ввод — поиск, Enter — открыть, Esc — назад.
Опасные действия требуют явного `yes`; пустой Enter не подтверждает.
Аргументы команд — в `--help`, Kubernetes-команды — в `kube-tools-help`.
`Option+A` открывает Tinycast; автозапуск включается в настройках приложения.

## Kubernetes

`dots-kube` — отдельный переносимый бинарник; нужны `kubectl`, kubeconfig и
его auth-плагины. `dots-tools kube` использует тот же код.

```sh
dots-kube setup                    # файл → context → namespace → подтверждение
dots-kube config                   # сохранённые подключения без секретов
dots-kube doctor                   # локальная диагностика
kpstage get pods
kpstage namespace                  # сменить только namespace
kpstage-logs api
kpstage-logs-multi                 # выбрать несколько подов
kpstage-rollout-restart            # выбрать workloads
kpprod-deploy-watch api
```

Окружения: dev/stage/preprod/prod. Мастер сохраняет привязки в
`~/.config/dots-kube/`, не переписывая исходные kubeconfig и обычный
`kubectl current-context`. Команды `kp…` закрепляют context/namespace только
для своего вызова. Изменения в prod требуют подтверждения; для скриптов
предусмотрен явный `KPROD_CONFIRMED=1`.

Логи: Space — пауза, `/` — поиск, Enter — детали, `m` — краткий вид.
По умолчанию видны полные записи; `logs-save` сохраняет исходный вывод.
`rollout-restart` перезапускает workloads, `pod-restart` — конкретные поды.
Релизы macOS/Linux собираются через `./scripts/build-kube-release.sh`.

## Pi и прокси

```sh
./install -c steps/pi.yml
pi-proxy --set-sub                 # первичная настройка VLESS-подписки
pi
pi-proxy --auto                    # заново выбрать быстрейший выход вне РФ
pi-proxy --status
```

`pi-proxy` управляет настоящим xray-core, а не реализует VLESS самостоятельно.
Запуск `pi` проверяет реальную страну выхода и доступность OpenAI.
Выбор по медиане трёх запросов кешируется на 30 минут; РФ и неизвестная
страна исключаются. Без проверенного маршрута Pi не запускается напрямую.
Это замер задержки до API, не скорости генерации. Смена сервера может оборвать
соединения; непрерывного переключения во время сессии нет.

Подписка и конфигурация — `~/.config/xray-pi/`. Системный VPN и proxy-настройки
не меняются. Shell-команды агента не наследуют proxy env; корпоративный MCP
обходит внешний туннель. После обновления обёртки перезапустите Pi;
в открытой zsh/bash: `source ~/.config/shell/pi-proxy.sh`.

MCP заданы в `config/pi/mcp.json`: Peekaboo управляет Mac, Excalidraw создаёт
диаграммы. Проверка: `pi mcp list`; после применения настроек — `/reload`.
Peekaboo нужны разрешения macOS на запись экрана, Accessibility и синтетический
ввод: `peekaboo permissions status`. Excalidraw отправляет данные удалённому
серверу; секреты ему не передавать. Встроенного холста в Pi нет.

## Herdr

Обёртка сама собирает клиент с локальным патчем вкладок для установленной версии.
Первой сборке нужны Rust/Cargo; проверенный Zig при необходимости скачивается
в кеш. При ошибке используется стандартный или подходящий готовый клиент.

```sh
scripts/build-herdr-ui.sh          # повторить сборку без ожидания cooldown
HERDR_DOTS_NO_PATCH=1 herdr        # запустить исходный клиент
```

Логи и клиенты: `~/.local/share/dots/herdr/<version>/`.
Для нового UI переподключите клиент (Ctrl+A, D → `herdr`), не останавливая сервер.

## Поиск и тесты проектов

```sh
jg auth
jg "Где проверяется авторизация?" ./src
pytest-safe -- uv run pytest -n auto
```

Jevgrep отправляет выбранные исходники провайдеру: сужайте корень, сохраняйте
фильтры секретов. В shell-инструментах агента используйте `~/dots/scripts/jg`
для настроенного прокси. `pytest-safe` снижает приоритет процесса;
xdist по умолчанию использует два worker'а, явные настройки имеют приоритет.

## Приватные настройки

```sh
dots private init                 # создать отсутствующие шаблоны
dots private check                # проверить без вывода значений
```

| Путь | Содержимое |
| --- | --- |
| `~/.config/dots/private.zsh` | Личные алиасы, endpoints и пути; [пример](config/zsh/private.example.zsh) |
| `~/.config/git/private.inc` | Имя/email Git и личные настройки |
| `~/.pi/agent/` | Авторизация и история Pi; вход — `/login` |
| `~/.config/jevgrep/credentials.json` | Доступ Jevgrep; вход — `jg auth` |
| `~/.kube/`, `~/.ssh/`, `~/.config/xray-pi/` | Подключения, ключи и подписка прокси |

Файлы с секретами — 600, приватные каталоги — 700. Храните их вне Git,
восстанавливайте из защищённого хранилища и проверяйте пути на новом Mac.
OAuth-вход выполняйте заново. Удаление файла не очищает историю Git.

## Обслуживание

```sh
dots-tools setup cleanup           # только план удаления подтверждённых остатков
dots-tools setup cleanup --apply   # применить план
```

Уборка не удаляет действующие managed links, приватные настройки, бэкапы,
релизы и зависимости, не обходит чужие каталоги. `make test-go`,
`make test-node`, `make test-runner` запускают отдельные группы проверок.

Дополнительно: [VPN](vpn/README.md) и
[браузерные группы задач](tools/task-tab-groups/README.md).
Разовые отчёты храните в `$TMPDIR`; справочник параметров — в `--help`,
а не в копиях CLI-документации. Файлы skills/instructions — рабочие инструкции
агентов, не архив, и устанавливаются вместе с конфигами.
