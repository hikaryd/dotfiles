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

# откатить конфиги: симлинки, Firefox/Zen/AyuGram/Claude/Codex, твики шелла и defaults
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
