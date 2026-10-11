# Рабочее окружение

Dotfiles для macOS в стиле **Ayu Dark**: Ghostty, Nushell/zsh, Herdr/tmux,
AeroSpace/Rift, Neovim, Yazi, LazyGit и CLI-агенты.
Пакеты — [`Brewfile`](Brewfile), настройки — [`config/`](config/).

## Установка и проверка

Нужны Xcode Command Line Tools, Homebrew и Go 1.26+. Репозиторий — `~/dots`:

```sh
./install
./scripts/build-go-tools.sh       # пересобрать собственные Go-утилиты
make test                        # Go race/vet, Node и runner
```

Runtime — `tools/dots`, бинарник `dots-tools`; ссылки — в игнорируемом `build/bin/`.
Bash связывает установку и приложения; Python нужен стороннему Dotbot, не нашим командам.
Отдельные проверки: `make test-go`, `make test-node`, `make test-runner`.
Откат: `./uninstall --dry-run` для просмотра плана, затем `./uninstall`.
Без `--packages` пакеты Homebrew не удаляются; конфиги переносятся в бэкап.

## Ежедневные команды

```sh
dots                             # меню с поиском и цветами Ayu
dots help
dots menu --preview --plain       # предпросмотр без запуска действий
dots-tools palette --help
dots-vpn --help
kpstage get pods                  # context и namespace закреплены
kpdev-logs api                    # выбор pod и поток логов
kpprod-deploy-watch api           # read-only dashboard
kube-tools-help
```

↑/↓ — выбор, Enter — открыть, Esc — отмена; в запасном gum — Ctrl+C или Esc ×2.
`NO_COLOR` отключает цвета; `--json` даёт чистый вывод для скриптов.
[VPS и роутер](vpn/README.md) настраиваются отдельно, не обычным `./install`.

## Kubernetes: один переносимый бинарник

`dots-kube` работает отдельно от dotfiles, `private.zsh`, zsh и gum. Нужны только
`kubectl`, kubeconfig и, если они указаны в kubeconfig, внешние auth-плагины.
Встроенный Ayu-интерфейс одинаков в zsh, Nushell и других оболочках.
`dots-tools kube …` использует тот же модуль.

На новом компьютере передайте бинарник нужной платформы и безопасно получите
kubeconfig у администратора. Затем:

```sh
./dots-kube setup
# 1. Выбрать найденные файлы: Space — отметить, Ctrl+A — все, Ctrl+O — другая папка.
# 2. Для нужных окружений выбрать ФАЙЛ → КОНТЕКСТ → NAMESPACE.
# 3. Проверить общий план и выбрать «Сохранить настройки».
# Устанавливает команды в ~/.local/bin; исходные kubeconfig не меняет.
export PATH="$HOME/.local/bin:$PATH"  # добавьте в конфиг своей оболочки, если нужно

dots-kube config                  # источники, окружения, current-context; без секретов
dots-kube doctor                  # локальная диагностика, без запросов к кластеру
kpstage get pods
```

Явная настройка одного окружения, в том числе без терминала:

```sh
./dots-kube setup stage --kubeconfig /path/to/stage.yaml \
  --context existing-context --namespace application --yes
# Для отдельного prod-файла повторите setup prod --kubeconfig /path/to/prod.yaml …
# --import FILE отдельно импортирует предоставленный конфиг в локальное хранилище.
# --skip-aliases оставляет только настройку; --bin-dir DIR меняет каталог команд.
```

**Несколько файлов — нормальный сценарий.** Мастер ищет kubeconfig в `~/.kube/`,
в том числе в каталогах `config/` и `configs/`, а также в текущем `KUBECONFIG`.
Выбираются только отмеченные файлы; названия файлов не определяют окружение.
Для каждого окружения выбирается конкретный файл и context. На шаге контекста
Ctrl+O позволяет изменить выбранные файлы, Ctrl+L очищает поиск; счётчик показывает
число совпадений и общий размер списка. Поиск нового окружения начинается пустым.
На шаге namespace **Ctrl+O** получает реальный список из выбранного кластера:
это явный read-only запрос, после которого можно выбрать имя вместо ручного ввода.
Если RBAC запрещает `list namespaces` или подключение недоступно, остаётся ручной
ввод; ограничение показывается в мастере. Если namespace в context отсутствует,
мастер требует выбрать или указать его, не подставляя `default`. Повторная настройка того же подключения сохраняет ранее подтверждённый
namespace. Одинаковые имена
cluster/user/context в разных файлах не смешиваются: `kpstage` и `kpprod` имеют
раздельные цепочки загрузки. Если context и его cluster/user лежат в разных
файлах, стандартное объединение тоже поддерживается — только без конфликтов.

Выбор сохраняется в `${XDG_CONFIG_HOME:-~/.config}/dots-kube/sources.yaml` —
**только пути, без ключей**. Контексты `dots-dev`, `dots-stage`, `dots-preprod`,
`dots-prod` хранятся в соседнем каталоге `contexts/` и содержат лишь ссылки на
cluster/user и выбранный namespace. Исходные kubeconfig, включая read-only,
не переписываются. Менять или экспортировать `KUBECONFIG` после мастера не нужно.
Сохранённые привязки используются алиасами; явный `--kubeconfig FILE` в setup/config
важнее сохранённого выбора. До первой настройки используются стандартные
`KUBECONFIG` / `~/.kube/config`. Обычный `kubectl` и его `current-context` не меняются.

Esc возвращает на предыдущий шаг; до подтверждения ничего не записывается.
На узком экране длинное объяснение прокручивается PgUp/PgDn, кнопки остаются доступны.
Повторный setup допустим; невыбранные окружения сохраняют прежние настройки.
Запись собственных файлов — через приватные временные файлы и атомарную замену
каждого файла с проверкой внешних изменений.

`kdev/kstage/kpreprod/kprod`, `kp…` и суффиксы сами выбирают окружение **для
своего вызова**, не переключая обычный `kubectl` в других терминалах. Неизвестная
или конфликтующая привязка — ошибка, без угадывания окружений. Target/auth overrides
запрещены; аргументы контейнера после `--` сохранены. Изменения в prod требуют
`yes` в терминале или deliberate `KPROD_CONFIRMED=1`.
Старые `KUBE_<ENV>_KUBECONFIG/CONTEXT/NAMESPACE` больше не используются.
`kpstage config` показывает подключение stage, без запуска kubectl;
`kpstage setup` настраивает только stage. **`kpstage namespace`** получает список
namespace read-only запросом и позволяет изменить только namespace уже сохранённого
подключения, без повторного выбора файлов/context. Изменение сохраняется после
отдельного подтверждения; следующие `kpstage-*` используют его автоматически.
Для списка без изменения: `kpstage namespace --list --json`. `config --json` и `doctor --json --strict`
удобны для проверок в скриптах.

### Логи

`kpstage-logs api` — читаемые отдельные записи: время, контейнер, сообщение,
визуальный разделитель. JSON-сообщение вынесено из метаданных; полный JSON доступен
в деталях. `kpstage-logs-multi api error` — компактная таблица нескольких подов
с постоянными колонками и подсветкой поиска. Изменяются только нужные строки
экрана, без очистки и прокрутки всего кадра; поток не задаёт частоту перерисовки.

- **Space / p** — пауза или возврат к живому потоку. На паузе снимок не меняется;
  новые записи продолжают приниматься в ограниченный буфер.
- **↑↓ / j/k** — выбор записи; **Enter** — полный текст, в том числе форматированный JSON.
- В деталях **↑↓ / PgUp / PgDn** — прокрутка, **←→** — соседняя запись.
- **/** — поиск, **f** — только совпадения, **c** — сброс; **Esc / q / Ctrl+C** — выход.

По умолчанию хранятся до 5000 записей и 32 MiB текста. Старые записи вытесняются;
счётчик виден в шапке. Пределы: `KUBE_MULTI_LOG_BUFFER`, `KUBE_LOG_MAX_BYTES`.
Очень большие записи ограничиваются только в UI; для полного исходного вывода —
`logs-save`. При выводе в pipe/файл TUI и цвета не добавляются; сохранённые логи
остаются исходными. `KUBE_LOG_DIR` меняет каталог сохранения.

### Сборка и предпросмотр

```sh
./scripts/build-go-tools.sh       # установить integrated runtime и dots-kube
./scripts/build-kube-release.sh   # macOS/Linux × arm64/amd64, плюс SHA256SUMS
./build/bin/dots-kube preview logs --plain
./build/bin/dots-kube preview selector --width 70 --height 20
```

Архивы в `build/releases/kube/` содержат бинарник и инструкцию без настроек и
секретов. Для отдельной сборки: `CGO_ENABLED=0 go -C tools/dots build -trimpath
-o ../../build/bin/dots-kube ./cmd/dots-kube`. Все аргументы — `dots-kube --help`.
`config/zsh/kube-tools.zsh` оставлен только для удаления прежних функций из уже
открытых zsh-сессий; он больше не меняет `KUBECONFIG`.

## Лаунчер macOS

`Option+A` открывает Tinycast. В строке поиска доступны калькулятор и конвертация
валют, например `100 USD to EUR` или `100 USD to RUB`; курсы справочные.
Установщик назначает хоткей через `dots-tools setup tinycast-hotkey`;
если Tinycast уже запущен, перезапустите его после изменения.
Автозапуск включается в Tinycast → Settings → General → Launch at login.

## Прокси Pi и shell-команды

Pi использует локальный прокси для запросов моделей и MCP. Его proxy-переменные
не должны наследоваться shell-командами: иначе `kubectl` и корпоративный `glab`
пытаются обращаться к внутренним серверам через внешний VLESS-прокси.
`shellCommandPrefix` в `config/pi/settings.json` очищает HTTP/WebSocket proxy env
только в дочерней оболочке, не меняя окружение самого Pi. Это действует для
инструмента `bash` и команд `!`/`!!`; нужный отдельной команде прокси можно задать явно.
После изменения `~/.pi/agent/settings.json` выполните `/reload` в Pi.

## Щадящий запуск pytest

Во всех обычных zsh-сессиях `PYTEST_XDIST_AUTO_NUM_WORKERS` по умолчанию равен `2`:
`pytest-xdist -n auto` не создаёт воркеры на все ядра. Настройка наследуется
дочерними процессами, включая `uv run pytest` и `python -m pytest`; явный `-n N`
или собственное значение переменной имеют приоритет. xdist не требуется для
проектов, которые его не используют, и параллельный запуск сам не включается.

Для фонового приоритета используйте обёртку:

```sh
pytest-safe -q                              # pytest из текущего PATH/venv
pytest-safe -- uv run pytest -n auto
pytest-safe -- python -m pytest -q
PYTEST_XDIST_AUTO_NUM_WORKERS=4 pytest-safe -- uv run pytest -n auto
```

На macOS обёртка использует `taskpolicy -b` и `nice -n 10`; без `taskpolicy` —
только `nice`. Это не жёсткая квота CPU и не ограничение потоков внутри тестов.
Обычные команды без `pytest-safe` автоматически получают только настройку xdist,
не фоновый приоритет. IDE, запущенные вне этой оболочки, и `zsh -f` не обязаны
читать `.zshenv`; сама обёртка задаёт значение по умолчанию и в этих случаях.
Для текущего терминала: `source ~/.zshenv; source "$DOTS_ROOT/config/zsh/commands.zsh"`.

## Приватные настройки

```sh
dots private init                # создать только отсутствующие шаблоны
dots private check               # проверить без вывода значений
```

`init` готовит четыре файла: оболочка, Git и MCP-секреты Claude/Codex.
Существующие файлы не перезаписываются. `check` проверяет их права и известные
незаполненные шаблоны, не все внешние хранилища и не работоспособность ключей.
Заполняйте файлы редактором, не командами с паролями в истории терминала.

| Где | Что хранится |
| --- | --- |
| `~/.config/dots/private.zsh` | Личные алиасы, API-адреса и пути; пример — `config/zsh/private.example.zsh` |
| `~/.config/git/private.inc` | Имя/email Git и личные `maintenance.repo`; include фиксирован в общем Git-конфиге |
| `~/.claude/.mcp-secrets` | `Z_AI_API_KEY`, `MAGIC_API_KEY`, `KUBECONFIG_PATH`, необязательный `ANTHROPIC_BASE_URL` |
| `~/.codex/.mcp-secrets` | `Z_AI_API_KEY`, необязательный `CF_OPENAI_BASE_URL` |
| `~/.pi/agent/` | `auth.json` и приватная история `sessions/`; вход — `/login` |
| `~/.abide/.env` | `OPENROUTER_API_KEY` для Abide/Jev; другие ключи — через `abide login` |
| `~/.kube/`, `~/.ssh/` | Kubeconfig, SSH-настройки и проверенные `known_hosts`; ключи предпочтительно через менеджер паролей/SSH-агент |
| `~/.config/xray-codex/` | Приватная подписка и конфигурация прокси |

Учитываются `DOTS_PRIVATE_ZSH`/`XDG_CONFIG_HOME`, `CLAUDE_CONFIG_DIR`,
`CODEX_CONFIG_DIR`/`CODEX_HOME`, `PI_CODING_AGENT_DIR`.
Файлы с секретами — `600`, приватные каталоги — `700`; необязательные интеграции
можно не заполнять. Настройки телефона и других отдельных проектов ведутся в их репозиториях.

На новом компьютере восстановите только нужные файлы из защищённого хранилища
или через SSH с проверкой ключа, поправьте пути и выполните `dots private check`.
После заполнения MCP: `bash scripts/claude-apply.sh` и/или `bash scripts/codex-apply.sh`;
они делают бэкапы. В OAuth-сервисы войдите заново, не копируйте автоматически токены,
cookies и сессии. Выполните `dots projects refresh`, чтобы пересобрать пути проектов.
Секреты и их бэкапы — вне Git; удаление из текущих файлов не очищает историю Git.

## Документация

Постоянные руководства — этот файл и `vpn/README.md`; аргументы команд — в `--help`.
Разовые аудиты и исследования храните в `$TMPDIR`, без отдельных README на каждый скрипт.
Файлы `config/*/instructions.md` и `config/codex/search-instructions.md` — рабочие инструкции
агентов, а не архив документации.
