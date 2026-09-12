# Dotfiles

```bash
cd ~/dots
./install
```

## Установка с нуля

1. Установите Xcode Command Line Tools:
```bash
xcode-select --install
```

2. Установите Homebrew:
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

3. Запустите установку:
```bash
./install
```

## Обновление конфигов

После изменения любых конфигов просто запустите:

```bash
cd ~/dots
./install
```

Большинство изменений применится через симлинки; смешанные конфиги Claude/Codex
обновляются их apply-шагами во время `./install`.

## Рабочее меню `dots`

```bash
dots                         # меню действий в текущем каталоге
dots projects open           # выбрать проект/worktree
dots projects refresh        # обновить приватный список из живых tmux panes и Git worktrees
dots style focus             # непрозрачный фон, без анимаций
dots style presentation      # крупнее, без путей/веток в prompt/status
dots style glass             # вернуть исходное оформление
dots doctor                  # read-only: OK / WARN / FAIL
```

Клавиши: **Ctrl+O** в zsh — меню; **Ctrl+A, O** в tmux — меню в popup;
**Ctrl+A, P** — проекты. Прежний **Ctrl+A, T** с sesh сохранён.
Меню содержит LazyGit, конфиги, слушающие TCP-порты и read-only Kubernetes
действия; оно не запускает deploy, kill, restart или scale.

Проекты определяются по реальным каталогам открытых панелей и зарегистрированным
worktree. Разные checkout внутри одной tmux-сессии остаются отдельными пунктами.
Существующие панели переиспользуются (редактор/агент предпочтительнее idle-shell),
без отправки ввода в работающий процесс. Новое пространство содержит окна
`editor`, `shell`, `logs`; `logs` — пустой shell, серверы автоматически не запускаются.
Список хранится только в `~/.config/dots/projects.json` с правами `600`.
`list` и открытие проверяют живые панели заново; повреждённый inventory не затирается.

### Ссылки из логов — `dots refs`

**Ctrl+A, F** открывает picker ссылок из исходной tmux-панели. Захватывается
последняя часть history (по умолчанию 200 строк плюс видимая область): HTTP(S)-URL,
UUID, request/correlation/trace-id и существующие `file:line`, включая Python
traceback. Относительные файлы ищутся от cwd исходной панели, а не popup.
После выбора отдельное действие `copy` или `open`; по умолчанию — отмена.
ID можно только скопировать; URL открывается в браузере, файл — в Neovim на строке.

```bash
dots refs --list                      # показать без copy/open
dots refs --pane %12 --lines 500       # конкретная панель
printf 'request-id=example-123\n' | dots refs --stdin --json
```

Захват ограничен 1 MiB и не сохраняется скриптом на диск; ANSI/OSC и небезопасные
пути отбрасываются. Ссылки могут содержать секреты в query: просмотр/копирование
не является автоматической редакцией секретов. Для чтения файлов/открытия URL
всегда нужен явный выбор действия.

### Преобразования clipboard — `dots clip`

**Ctrl+A, Y** — меню: `json` (pretty), `compact`, `base64` (decode UTF-8),
`jwt` (decode), `time`, `redact`. По умолчанию сначала ограниченный безопасный
preview, затем подтверждение `yes` перед заменой clipboard. `--stdout` не пишет
в clipboard; `--write` означает явную запись без вопроса.

```bash
dots clip json                       # preview → yes → clipboard
dots clip jwt --stdout               # НЕ проверяет подпись/доверие, UNVERIFIED
dots clip redact --keys token,password,authorization
printf '1789200000000' | dots clip time --stdin --stdout
```

В `time` числа с модулем ≥ `1e11` считаются epoch milliseconds, меньшие — seconds;
ISO 8601 требует timezone. `redact` рекурсивно маскирует точные имена ключей без
учёта регистра (default: token, secret, password, authorization, api_key),
но не гарантирует удаления всех секретов. JWT здесь только отладочно декодируется.

Лимит 1 MiB, ошибки/cancel не вызывают `pbcopy`. Перед записью повторно проверяются
текст и AppKit `changeCount`: новая копия с прежним текстом тоже отменяет операцию.
Это best effort, не атомарный compare-and-swap: между последней проверкой и
`pbcopy` остаётся короткий промежуток. Явная запись заменяет rich clipboard обычным
текстом. Скрипт не хранит clipboard в файлах и не отправляет его в сеть; сторонний
clipboard manager/Universal Clipboard управляются отдельно.

### Общий экран проектов — `dots status`

**Ctrl+A, V** — Git branch, staged/unstaged/untracked/conflicts, процессы открытых
tmux-панелей и TCP listeners. В интерактивном экране `r`/Enter обновляет снимок,
`o` открывает выбранный проект через существующий безопасный picker, `q` выходит.
Автоматического polling нет. По умолчанию видны проекты с открытыми панелями или
атрибутированными listeners; это не показатель текущей CPU-активности.

```bash
dots status                         # один текстовый снимок
dots status --all --json             # также известные неоткрытые worktree
```

Порты сопоставляются по cwd процесса, при недоступности cwd — по дереву процессов
панели. Неопределённые listeners считаются отдельно; `LISTEN` не доказывает
готовность HTTP/API. Полные command argv/env не собираются. Git status работает
без optional index locks/fsmonitor; если настроены исполняемые clean/process
filters, состояние явно `unknown`, а не запуск произвольного фильтра. Ошибка или
таймаут тоже не подменяется `clean`. Реестр и рабочие деревья не изменяются.

### Профили оформления

Профили хранятся в `~/.config/dots/style` и `ghostty-style.conf`, не меняя базовую
палитру. Ghostty мягко перечитывает конфиг через штатный AppleScript API;
при недоступности API команда явно сообщает, что нужен Reload Config.
Neovim применяет профиль при следующем FocusGained/смене окна, zsh — на следующем
prompt. **Presentation не скрывает содержимое буфера, scrollback и вывод команд**;
это не режим защиты секретов при демонстрации экрана. Исходное оформление
возвращается через `dots style glass`, пользовательские изменения status options
между переключениями сохраняются.

### Локальная история и уведомления

**Ctrl+R** открывает Atuin. Up и другие fzf-клавиши сохранены; autosuggestions
по-прежнему используют быстрый zsh history backend. В конфиге выключены sync,
update checks и daemon, аккаунт не нужен. Enter выбирает команду для редактирования,
не выполняя её сразу. Команды с начальным пробелом исключены из Atuin;
фильтр секретов не заменяет осторожность при вводе credentials. История не хранится
в Git. Импорт старой zsh history: `atuin import zsh` (исходный файл не удаляется).

Команды длительнее 30 секунд посылают Ghostty уведомление с результатом,
длительностью и именем каталога; текст команды и аргументы не передаются.
В presentation имя каталога тоже скрыто. В tmux доставка работает и из скрытых
панелей через tty подключённого Ghostty-клиента, без глобального `passthrough=all`.
Активная панель при переднем Ghostty не уведомляет; вне tmux подавление проверяет
активное приложение, а не конкретный tab. Без подключённого Ghostty-клиента
tmux-уведомления не доставляются. `DOTS_NOTIFY=0` отключает hook.
`dots notify --test` проверяет транспорт; macOS Focus/разрешения управляют баннером.
Последний результат доставки без текста команды — `~/.local/state/dots/last-notification.json`.

Zsh hooks/клавиши подключаются в **новых интерактивных shell**. Установка не
перезапускает существующие панели, процессы или редакторы.

### Проверка установки

`dots doctor --json` выдаёт машинный отчёт; `--strict` считает предупреждения
неуспехом. Обычный doctor возвращает ненулевой статус только при FAIL и ничего не
исправляет сам. Полный `./install` теперь выводит сводку всех шагов и ненулевой
exit при ошибках, продолжая остальные шаги; `./install -c steps/terminal.yml`
по-прежнему выполняет только выбранный конфиг. Не нужно запускать полный install
только ради переключения профиля или обновления списка проектов.

Зависимости существующих integrations включены в `Brewfile`: `eza` для preview
каталогов Yazi и `hyperfine` для `nvim-bench` в zsh/Nushell. Старые алиасы
`deploy-dev` удалены вместе со ссылкой на уже удалённый скрипт: прежняя автоматизация
merge/push/tag не восстанавливается. Деплой остаётся ответственностью конкретного проекта.

Локальная регрессия: `python3 -m unittest discover -s tests -v`.
Kubernetes tests используют fake kubectl, не обращаясь к кластерам;
tmux tests поднимают отдельные серверы, не затрагивая рабочие сессии.

## Waterfox

Waterfox устанавливается через `Brewfile`; окна направляются на рабочее
пространство 1. После первого запуска браузера `./install` применяет тёмную тему
и восстановление предыдущей сессии. Отдельное применение:

```bash
python3 scripts/waterfox-apply.py
```

Скрипт обрабатывает только существующие, уже использованные профили из
`~/Library/Application Support/Waterfox/profiles.ini`. Личные настройки вне
блока dots в `user.js` сохраняются; перед изменением создаётся соседняя копия
`user.js.dots-backup-*`. Отсутствующие профили не создаются. Новые настройки
вступают в силу после перезапуска браузера. Вкладки, расширения и авторизации
не хранятся в репозитории и этим шагом не переносятся.

## Приватные machine-local настройки

Git identity хранится отдельно от репозитория:

```ini
# ~/.config/git/private.inc (chmod 600)
[user]
    name = Your Name
    email = you@example.test
```

Приватный OpenAI-compatible provider для `scripts/ai_helper` настраивается в
`~/.config/ai-helper/openai-compatible.json` (также `chmod 600`) ключами
`api_key`, `api_base` и `model`. Рабочие endpoints, identity и service paths в
tracked-файлы добавлять не нужно.

В LazyGit `Shift+C` сначала генерирует сообщение по staged diff, затем открывает
его в `nvim`. При ошибке provider пустой commit editor не открывается: причина
печатается в терминал и сохраняется с mode `600` в
`~/.local/state/ai-helper/last-error.log`. `Shift+M` открывает обычный commit без
AI.

## Kubernetes

Реальные kubeconfig/context/namespace не хранятся в репозитории. Один раз
создайте machine-local файл и ограничьте доступ к нему:

```bash
mkdir -p ~/.config/dots
cp ~/dots/config/zsh/private.example.zsh ~/.config/dots/private.zsh
chmod 600 ~/.config/dots/private.zsh
$EDITOR ~/.config/dots/private.zsh
```

После заполнения файла и загрузки `~/.zshrc` доступны привязанные к окружению
команды:

```bash
kpdev get pods             # обычный kubectl в настроенном dev namespace
kpdev-logs                 # выбрать pod и подписаться на логи всех контейнеров
kpdev-logs-save            # выбрать pod и сохранить все доступные логи в файл
kpdev-logs-multi           # live-логи всех pod по glob с runtime highlight/filter
kpdev-cron-run             # ACTION: создать разовый Job из выбранного CronJob
kpdev-exec                 # ACTION: выполнить команду внутри pod/workload
kpdev-log-search           # поиск literal-строки по логам подходящих pod
kpdev-jobs                 # выбрать Job из истории и открыть логи всех его pod
kpdev-jobs-save            # выбрать Job/CronJob и сохранить логи всех pod/container
kpdev-deploy-watch         # read-only наблюдение за раскаткой workload
kpdev-scale                # ACTION: посмотреть и изменить число реплик workload
kpdev-pod-analyze          # один или несколько pod: состояние, ресурсы и процессы
kpdev-pod-restart          # ACTION: удалить pod, дождаться replacement и открыть логи
```

Все команды симметричны для `dev`, `stage`, `preprod` и `prod`: например,
`kpreprod get nodes`, `kppreprod get pods`, `kppreprod-logs api` и
`kppreprod-deploy-watch api`.

`*-deploy-watch`, `*-logs`, `*-logs-multi` и `*-jobs` выполняют только чтение
(`get`/`logs`);
они не вызывают `kubectl rollout`, `apply`, `patch`, `delete` или `restart`.

Полная доступная из kubectl аналитика одного или нескольких выбранных pod:

```bash
kpdev-pod-analyze worker
kpstage-pod-analyze api
kppreprod-pod-analyze api
kpprod-pod-analyze api
```

В `gum` pod’ы и реплики workload отмечаются `Tab` (`Ctrl+A` выбирает все) и
подтверждаются `Enter`; `Space` остаётся обычным символом строки поиска. Без
`gum` номера вводятся через запятую. Для нескольких pod dashboard группирует
реплики по верхнему workload, показывает компактную цветную сводку по группе и
одну строку на обычный single-container pod. Вложенные строки добавляются только
для pod с несколькими контейнерами; повторяющиеся предупреждения
схлопываются со счётчиком, а обычные успешные Events не засоряют экран.
Dashboard показывает состояние и Ready каждого контейнера, рестарты и последний
exit/OOMKilled, текущие CPU/RAM относительно requests/limits, QoS, node, последние
Warning events и best-effort список наиболее нагруженных процессов внутри
контейнера. В таблице блоки разделены вертикальными линиями: `CPU NOW`,
`CPU/REQ`, `CPU/LIM` относятся только к CPU, а `RAM NOW`, `RAM/REQ`, `RAM/LIM` —
только к памяти. `NOW` — текущее потребление, `/REQ` — доля request, `/LIM` —
доля limit; значения около лимита подсвечиваются отдельно от lifecycle-состояния
pod.

Для процессов не нужен `ps`: фиксированный read-only shell-сборщик читает
`/proc/stat`, `/proc/<pid>/stat`, `/proc/<pid>/status` и `/proc/<pid>/comm` через
`kubectl exec`. CPU процесса считается по дельте двух выборок, поэтому на первом
снимке виден `sample`, а примерно через пять секунд — процент одного CPU core
(`200%` означает два полностью занятых core). `LIM-MEM%` показывает RSS процесса
относительно memory limit контейнера; RSS разных процессов может учитывать общие
страницы повторно. Для образов без `sh`/доступного procfs одна общая строка
сообщит, что process snapshot недоступен, а повторная проверка будет отложена на
пять минут. Остальная аналитика продолжит работать. Команда не изменяет
Kubernetes-объекты и
в prod не запрашивает подтверждение. Для одиночного pod также показываются
avg/p95/max и Unicode-графики. Снимки сохраняются между запусками в
`~/.local/state/kube-tools/pod-metrics` и группируются по верхнему controller,
поэтому история переживает замену pod. По умолчанию показываются 24 часа, а
хранятся 30 дней. В multi-pod режиме история не смешивает параллельные реплики и
показывается только текущее состояние.

В multi-pod таблице `STATE` отражает только текущий lifecycle (`READY`,
`NOT READY`, `PENDING`, `FAILED`): превышение request/limit и Warning Events
показываются отдельными секциями и не превращают Ready pod в `ERROR`. События
старше десяти минут из текущего dashboard скрываются. Live-режим включается явно
shell-wrapper’ом и перерисовывает экран каждые `100ms`, даже если терминал не был
распознан как обычный TTY. Если `ISSUES`, события и process attribution вместе
не помещаются по высоте, второстепенные строки схлопываются с пометкой
`more rows hidden`, а верхняя pod-таблица остаётся на экране.

Настройки: `KUBE_POD_ANALYZE_REFRESH=100ms`, `KUBE_POD_ANALYZE_WINDOW=24h`,
`KUBE_POD_ANALYZE_RETENTION=720h`, `KUBE_POD_ANALYZE_CHART_POINTS=72`,
`KUBE_POD_ANALYZE_HISTORY_DIR=<path>`, `KUBE_POD_ANALYZE_PROCESSES=false`,
`KUBE_POD_ANALYZE_PROCESS_REFRESH=5s`. Для одного текстового снимка задайте
`KUBE_POD_ANALYZE_ONCE=1`.

`kubectl top`/metrics-server предоставляет только CPU и RAM в момент опроса,
поэтому история начинается после первого запуска команды и хранится локально.
CPU throttling, сеть, disk I/O и более ранняя серверная история требуют
Prometheus/cAdvisor либо другого metrics backend; dashboard явно отмечает эту
границу, а не подменяет отсутствующие данные предположениями.
Если локальный private-конфиг watcher-группы заполнен, первым пунктом picker идёт
`configured-suite`: он одним экраном объединяет статусы, pod lifecycle и
live-логи перечисленных workloads. Отдельный workload по-прежнему можно выбрать
ниже. Состав группы хранится вне Git, по одному Kubernetes workload name на
строку:

```text
~/.config/kube-tools/deploy-watch.resources
```

Пустые строки и строки с `#` в начале игнорируются. Альтернативный путь задаётся
через `KUBE_DEPLOY_WATCH_RESOURCES_FILE`. Состояние обновляется каждые `100ms`;
интервал можно переопределить через `KUBE_DEPLOY_WATCH_REFRESH`.

Потоковые логи нескольких pod:

```bash
kpstage-logs-multi 'api*'
kpstage-logs-multi 'api*' 'error request-id'
```

Внутри TUI:

- `/` — открыть заметную строку `FILTER WORDS` внизу и ввести слова;
- `Enter` — применить, `Esc` — отменить редактирование;
- `f` — показывать только строки, содержащие **все** введённые слова;
- `c` — очистить слова и фильтр;
- `Space` или `p` — зафиксировать/возобновить экран; новые логи продолжают
  складываться в отдельный buffer и не двигают paused viewport;
- `j`/`k` или `↑`/`↓` — выбрать строку на паузе;
- `Enter` на паузе — открыть полную строку с pod/container metadata и переносами;
- `q` — выйти.

Длинные строки в `logs-multi` и в секции `LIVE LOGS` у `deploy-watch`
переносятся на следующие экранные строки и больше не заменяются горизонтальным
`…`. Обычные `*-logs`, `*-jobs` и сохранённые raw-файлы по-прежнему выводят
исходную строку без преобразования.

Поток собирается параллельно из всех совпавших pod и всех контейнеров. Имена pod,
timestamps и severity раскрашиваются отдельно. В основном списке используется
короткий уникальный суффикс pod; повторяющийся kubectl pod/container prefix
удаляется и доступен в detail-режиме. Размер буфера задаётся через
`KUBE_MULTI_LOG_BUFFER` (по умолчанию `5000` строк), первоначальный tail —
через `KUBE_LOG_TAIL`.

Дополнительные акценты: duration/latency — cyan, HTTP method — blue,
`2xx/3xx/4xx/5xx` — green/cyan/yellow/red. Python traceback frames показываются
приглушённо-красным, финальная строка исключения — ярко-красным.

Action-команды названы явно: `*-cron-run` создаёт Job, `*-scale` изменяет
desired replicas, а `*-exec` выполняет
переданную команду внутри выбранного pod/workload. `*-pod-restart` удаляет
один или несколько выбранных pod под управлением ReplicaSet, StatefulSet,
DaemonSet или ReplicationController. В `gum` pod’ы отмечаются `Tab`, все видимые
выбираются через `Ctrl+A`, результат подтверждается `Enter`; `Space` вводится в
поиск. Без `gum` номера вводятся через запятую. Перезапуски
выполняются последовательно: команда ждёт новый UID и Ready перед переходом к
следующему pod, а после успеха открывает объединённые логи всех replacements.
Голые pod и pod других типов не удаляются. Остальные команды остаются read-only
наблюдателями.

Изменение числа реплик доступно для Deployment и StatefulSet. Picker сразу
показывает desired/current/ready/available; после выбора можно ввести новое
неотрицательное число. Число также можно передать вторым аргументом:

```bash
kpdev-scale api 3
kpstage-scale worker 5
kpprod-scale api 4  # потребует prod-подтверждение
```

После `kubectl scale` команда печатает точную команду `deploy-watch` для
наблюдения за достижением нового состояния. DaemonSet намеренно не предлагается:
число его pod определяется подходящими node, а не replicas.

```bash
kpdev-pod-restart worker
kpstage-pod-restart api
kpprod-pod-restart api  # потребует prod-подтверждение
```

Ожидание replacement по умолчанию ограничено 300 секундами. Настройки:

- `KUBE_POD_RESTART_TIMEOUT=600` — timeout в секундах;
- `KUBE_POD_RESTART_POLL_INTERVAL=2` — частота обновления lifecycle;
- `KUBE_POD_RESTART_FOLLOW_LOGS=0` — завершиться после Ready, не открывая логи.

Timeout и poll interval должны быть положительными целыми числами. Неверное
значение завершает команду до первого вызова `kubectl`.

`*-cron-run` сохраняет raw-логи всех pod’ов созданного Job в
`~/Documents/kube-logs/<environment>-<job>.log`, одновременно показывая
раскрашенный поток в терминале. Каталог можно переопределить через
`KUBE_LOG_DIR`; новые файлы создаются с правами `600`. Если `kubectl logs`
временно отвечает `ContainerCreating` или `PodInitializing`, команда повторяет
подключение до `KUBE_JOB_WAIT` (по умолчанию `2m`) вместо преждевременного
завершения. Интервал задаётся `KUBE_JOB_LOG_RETRY_INTERVAL`, короткий timeout
одной попытки — `KUBE_JOB_LOG_ATTEMPT_WAIT`.

Для уже завершившихся CronJob используется отдельная read-only команда:

```bash
kpdev-jobs-save nightly-cleanup
kpstage-jobs-save report-job
```

Она показывает историю Job newest-first, позволяет фильтровать по имени Job или
родительского CronJob и сохраняет все доступные логи всех pod/container с
timestamps в файл вида
`~/Documents/kube-logs/dev-job-<job>-YYYYMMDD-HHMMSS.log`. Для одного обычного
pod есть симметричная команда `kpdev-logs-save [filter]`. Обе команды показывают
логи в терминале одновременно с записью raw-версии в файл; суффикс окружения
можно заменить на `stage`, `preprod` или `prod`.

Поиск по логам всех pod’ов, подходящих под glob имени:

```bash
kpstage-log-search 'api*' 'message-id'
```

Без аргументов pattern и literal-строка запрашиваются интерактивно. Поиск
выполняется параллельно по всем контейнерам, захватывает current и доступные
previous logs, добавляет timestamp/pod/container prefix и использует literal
matching, а не regex. Настройки:

- `KUBE_LOG_SEARCH_SINCE=24h` — ограничить временное окно;
- `KUBE_LOG_SEARCH_PREVIOUS=0` — не читать previous container logs;
- `KUBE_LOG_SEARCH_PARALLEL=6` — число одновременных запросов.

Parallelism должен быть положительным целым числом. Ошибка любого основного
`kubectl logs` сохраняет его ненулевой exit status; отсутствие optional previous
logs остаётся допустимым и не отменяет результаты current container.

Команда видит только логи, которые ещё хранятся на Kubernetes nodes. Для
исторического поиска после удаления pod или ротации нужен Loki/Elasticsearch.

Полностью интерактивный запуск:

```bash
kpstage-exec
```

Сначала выбирается pod, Deployment, StatefulSet или DaemonSet, затем режим и
рабочая директория:

- `bash` — интерактивный shell через `kubectl exec -it`;
- `python` — multiline-редактор `gum write`. Код можно вставить целиком;
  `Enter` запускает, `Ctrl+J` добавляет строку, `Ctrl+E` открывает `$EDITOR`.
- директория — container `WORKDIR`, `..`, `../..` или произвольный путь.

Перед передачей в `python -` общий отступ автоматически удаляется, а вложенные
Python-отступы сохраняются. Для нестандартной директории сначала безопасно
выполняется `cd`, затем запускается Bash или Python; путь передаётся отдельным
аргументом и не интерполируется в shell-команду.

Явный target и команда по-прежнему поддерживаются:

```bash
kpstage-exec deploy/api -- python -
```

Можно также указать только target, а режим выбрать после:

```bash
kpstage-exec deploy/api
```

Суффикс окружения можно заменить на `stage`, `preprod` или `prod`, например
`kpstage-deploy-watch`, `kppreprod-logs` и `kpprod-logs`. Read-only команды в
prod не требуют подтверждения; `yes` запрашивается только для action/mutating
операций.
Необязательный аргумент сразу заполняет фильтр: `kpdev-logs worker`.
Например, `kpstage-jobs report-job` показывает подходящие Job newest-first
с локальным временем запуска, статусом и родительским CronJob.

Интерактивный выбор использует `gum`. Read-only deployment watcher собирается из
`tools/kube-rollout` без внешних Go-зависимостей и устанавливается в
`~/.local/bin/kube-deploy-watch` шагом `steps/terminal.yml`.
Обычные и CronJob-логи подсвечивают pod/container, timestamp, severity,
HTTP-методы и status codes. Стандартная переменная `NO_COLOR` отключает цвета.

## Установка отдельных компонентов

Вы можете установить только определенные компоненты:

```bash
# Только brew пакеты
./install -c steps/dependencies.yml

# Только shell конфиги
./install -c steps/terminal.yml

# Только редакторы
./install -c steps/editors.yml

# Только CLI tools
./install -c steps/tools.yml

# Только личный конфиг Codex
./install -c steps/codex.yml

# Только macOS настройки
./install -c steps/macos.yml
```

Экспорт, секреты и правила владения Codex-настройками описаны в
[`config/codex/README.md`](config/codex/README.md).

Настройки macos
```
./scripts/macos-defaults.sh
```

## Откат установки

```bash
# посмотреть, что будет сделано (ничего не меняет)
./uninstall --dry-run

# откатить конфиги: симлинки, Zen/Claude/Codex, твики шелла и defaults
./uninstall

# то же + удалить пакеты из Brewfile
./uninstall --packages
```

Удаляются только симлинки, указывающие в этот репозиторий. Скопированные файлы
переезжают в `~/dots-uninstall-backup-<дата>/`; смешанный Codex config сначала
копируется туда целиком, затем из оригинала снимаются только управляемые ключи.
Все действия перечисляются в `MANIFEST.txt` — ничего не теряется.

## Аудит лишнего

Что установлено в системе мимо Brewfile (кандидаты на чистку):

```bash
./scripts/audit-extras.sh
```
