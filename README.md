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

## Podbor Kubernetes

После загрузки `~/.zshrc` доступны привязанные к окружению команды:

```bash
kpdev get pods             # обычный kubectl в namespace podbor-dev
kpdev-logs                 # выбрать pod и подписаться на логи всех контейнеров
kpdev-logs-save            # выбрать pod и сохранить все доступные логи в файл
kpdev-logs-multi           # live-логи всех pod по glob с runtime highlight/filter
kpdev-cron-run             # ACTION: создать разовый Job из выбранного CronJob
kpdev-exec                 # ACTION: выполнить команду внутри pod/workload
kpdev-log-search           # поиск literal-строки по логам подходящих pod
kpdev-jobs                 # выбрать Job из истории и открыть логи всех его pod
kpdev-jobs-save            # выбрать Job/CronJob и сохранить логи всех pod/container
kpdev-deploy-watch         # read-only наблюдение за раскаткой workload
kpdev-pod-analyze          # CPU/RAM, состояние, события, история и графики pod
kpdev-pod-restart          # ACTION: удалить pod, дождаться replacement и открыть логи
```

`*-deploy-watch`, `*-logs`, `*-logs-multi` и `*-jobs` выполняют только чтение
(`get`/`logs`);
они не вызывают `kubectl rollout`, `apply`, `patch`, `delete` или `restart`.

Полная доступная из kubectl аналитика выбранного pod:

```bash
kpdev-pod-analyze worker
kpstage-pod-analyze service-api
kpprod-pod-analyze service-api
```

Dashboard показывает состояние и Ready каждого контейнера, рестарты и последний
exit/OOMKilled, текущие CPU/RAM относительно requests/limits, QoS, node, последние
Warning events, а также avg/p95/max и Unicode-графики. Он остаётся read-only и в
prod не запрашивает подтверждение. Снимки сохраняются между запусками в
`~/.local/state/podbor-kube/pod-metrics` и группируются по верхнему controller,
поэтому история переживает замену pod. По умолчанию показываются 24 часа, а
хранятся 30 дней.

Настройки: `KUBE_POD_ANALYZE_REFRESH=5s`, `KUBE_POD_ANALYZE_WINDOW=24h`,
`KUBE_POD_ANALYZE_RETENTION=720h`, `KUBE_POD_ANALYZE_CHART_POINTS=72`,
`KUBE_POD_ANALYZE_HISTORY_DIR=<path>`. Для одного текстового снимка задайте
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
~/.config/podbor-kube/deploy-watch.resources
```

Пустые строки и строки с `#` в начале игнорируются. Альтернативный путь задаётся
через `KUBE_DEPLOY_WATCH_RESOURCES_FILE`.

Потоковые логи нескольких pod:

```bash
kpstage-logs-multi 'service-api*'
kpstage-logs-multi 'service-api*' 'error request-id'
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

Поток собирается параллельно из всех совпавших pod и всех контейнеров. Имена pod,
timestamps и severity раскрашиваются отдельно. В основном списке используется
короткий уникальный суффикс pod; повторяющийся kubectl pod/container prefix
удаляется и доступен в detail-режиме. Размер буфера задаётся через
`KUBE_MULTI_LOG_BUFFER` (по умолчанию `5000` строк), первоначальный tail —
через `KUBE_LOG_TAIL`.

Дополнительные акценты: duration/latency — cyan, HTTP method — blue,
`2xx/3xx/4xx/5xx` — green/cyan/yellow/red. Python traceback frames показываются
приглушённо-красным, финальная строка исключения — ярко-красным.

Action-команды названы явно: `*-cron-run` создаёт Job, а `*-exec` выполняет
переданную команду внутри выбранного pod/workload. `*-pod-restart` удаляет
выбранный pod под управлением ReplicaSet, StatefulSet, DaemonSet или
ReplicationController, ждёт новый UID и состояние Ready, а затем подписывается
на его логи. Голые pod и pod других типов не удаляются. Остальные команды
остаются read-only наблюдателями.

```bash
kpdev-pod-restart worker
kpstage-pod-restart service-api
kpprod-pod-restart service-api  # потребует prod-подтверждение
```

Ожидание replacement по умолчанию ограничено 300 секундами. Настройки:

- `KUBE_POD_RESTART_TIMEOUT=600` — timeout в секундах;
- `KUBE_POD_RESTART_POLL_INTERVAL=2` — частота обновления lifecycle;
- `KUBE_POD_RESTART_FOLLOW_LOGS=0` — завершиться после Ready, не открывая логи.

`*-cron-run` сохраняет raw-логи всех pod’ов созданного Job в
`~/Documents/kube-logs/<environment>-<job>.log`, одновременно показывая
раскрашенный поток в терминале. Каталог можно переопределить через
`KUBE_LOG_DIR`; новые файлы создаются с правами `600`.

Для уже завершившихся CronJob используется отдельная read-only команда:

```bash
kpdev-jobs-save request-creation
kpstage-jobs-save security-checker
```

Она показывает историю Job newest-first, позволяет фильтровать по имени Job или
родительского CronJob и сохраняет все доступные логи всех pod/container с
timestamps в файл вида
`~/Documents/kube-logs/dev-job-<job>-YYYYMMDD-HHMMSS.log`. Для одного обычного
pod есть симметричная команда `kpdev-logs-save [filter]`. Обе команды показывают
логи в терминале одновременно с записью raw-версии в файл; суффикс окружения
можно заменить на `stage` или `prod`.

Поиск по логам всех pod’ов, подходящих под glob имени:

```bash
kpstage-log-search 'service-api*' 'message-id'
```

Без аргументов pattern и literal-строка запрашиваются интерактивно. Поиск
выполняется параллельно по всем контейнерам, захватывает current и доступные
previous logs, добавляет timestamp/pod/container prefix и использует literal
matching, а не regex. Настройки:

- `KUBE_LOG_SEARCH_SINCE=24h` — ограничить временное окно;
- `KUBE_LOG_SEARCH_PREVIOUS=0` — не читать previous container logs;
- `KUBE_LOG_SEARCH_PARALLEL=6` — число одновременных запросов.

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
kpstage-exec deploy/service-api -- python -
```

Можно также указать только target, а режим выбрать после:

```bash
kpstage-exec deploy/service-api
```

Суффикс окружения можно заменить на `stage` или `prod`, например
`kpstage-deploy-watch` и `kpprod-logs`. Read-only команды в prod не требуют
подтверждения; `yes` запрашивается только для action/mutating операций.
Необязательный аргумент сразу заполняет фильтр: `kpdev-logs worker`.
Например, `kpstage-jobs security-checker` показывает подходящие Job newest-first
с локальным временем запуска, статусом и родительским CronJob.

Интерактивный выбор использует `gum`. Read-only deployment watcher собирается из
`tools/podbor-rollout` без внешних Go-зависимостей и устанавливается в
`~/.local/bin/podbor-deploy-watch` шагом `steps/terminal.yml`.
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
