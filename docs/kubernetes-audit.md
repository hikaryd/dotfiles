# Аудит Kubernetes CLI и native UI

Область: `tools/dots/internal/kube`, standalone `cmd/dots-kube`, integrated dispatch,
shared TUI, sources/setup/namespace, workflow selectors, logs, exec, Jobs/CronJobs,
scale/restart, deployment/pod dashboards и упаковка. Проверка опирается на просмотр
кода, публичные CLI-тесты и реальные PTY с синтетическим kubectl.
**Настоящие кластеры не опрашивались; настоящие мутации не выполнялись.**

Это отчёт о проверенных сценариях, не гарантия отсутствия всех возможных ошибок.

## Найдено и исправлено

| Проблема | Причина и исправление | Проверка |
|---|---|---|
| Stage показывает namespace preprod после переключения | Context selector всегда выделял первую строку общего списка. Теперь выбирается сохранённая привязка конкретного окружения; список namespace показывает файл/context/cluster | PTY: preprod → Ctrl+O → stage → Ctrl+O; разные ответы; проверяются оба argv/KUBECONFIG |
| Теряется выбор context внутри одного файла | Сохранялись только пути, а несколько context могли иметь одинаковые cluster/user. Профиль теперь запоминает имя исходного context; изменение только namespace его не стирает | PTY: два context в одном файле, повторный setup и namespace change |
| Setup с относительным путём сохраняет нерабочие настройки | Wizard писал относительный путь, а loader требует абсолютный. Путь нормализуется до выбора/записи | PTY setup → fresh-process config --json |
| `logs-multi` без аргументов не позволяет выбирать поды | Открывался Input для шаблона. Восстановлен searchable multiselect; явные шаблоны по-прежнему автоматически выбирают совпадения | PTY: список → поиск → Space → смена поиска → Space → follow именно двух подов |
| Space перестаёт отмечать после поиска | Space считался текстом при непустом query. В multiselect это теперь toggle и при активном query; текущая строка показывает checkbox | Тот же PTY-сценарий, два разных поиска |
| Enter не отправляет скрытые поиском отметки | Проверка пустого visible списка игнорировала ранее выбранные строки. Отмеченное отправляется независимо от текущего фильтра | PTY: mark → query с нулём совпадений → Enter |
| Отмена namespace lookup теряет введённый текст | Черновик Input отбрасывался при Ctrl+O. Теперь сохраняется перед дополнительным диалогом | PTY: draft → Ctrl+O → Esc → исходный draft |
| Multi-log metadata обрезается даже на широком терминале | Колонки pod/container были жёстко ограничены 10/8 cells. Добавлены adaptive widths, RECEIVED/POD/CONTAINER/LEVEL/MESSAGE, стабильные pod tones, перенос сообщений | Wide PTY с длинными именами; narrow layout; no full-frame repaint |
| Логи выглядят сильно короче оригинала | JSON по умолчанию превращался только в message, а multi text обрезался по ширине. Default теперь FULL RECORDS, summary включается явно клавишей m; длинный текст переносится | Single/multi PTY: дополнительные JSON-поля видны; opt-in compact и полный detail |
| Enter в LIVE не открывает обещанные details | Обработчик работал только после ручной паузы. Enter теперь сам замораживает snapshot и открывает детали | PTY Enter непосредственно из LIVE |
| Forced color декорирует piped single logs | KUBE_COLOR=always обходил TTY-проверку. Цвет разрешён только при TTY и разрешённой теме | CLI byte-for-byte assertion |
| Piped multi logs меняют байты и теряют failure status | В pipe попадала UI-нормализация; stderr превращался в log records; helper возвращал 0. Headless stream теперь отдельный: raw stdout, separate stderr, kubectl exit status | CLI: tab/prefix/final record, 0/17 exit status, optional filtering |
| Pod analytics отвергает успешный JSON с Warning | CombinedOutput смешивал stdout и stderr перед JSON decode. Потоки разделены | CLI fixture с успешным JSON и stderr deprecation warning |
| q/Esc не работают во время initial pod fetch | Keyboard/raw lifecycle запускался только после инициализации. Теперь клавиатура подключается до первого запроса | PTY с blocked initial fetch: q отменяет subprocess |
| `cron-run` падает на CronJob.status.active | У Job active — integer; у CronJob active — ObjectReference[]. Общая модель принимала только int. Decoder поддерживает обе схемы | CLI с настоящей формой active refs + существующие Job/Cron retry/save tests |

Ctrl+O и в Select, и в Input возвращает `ErrAction`; служебных отрицательных
индексов больше нет. Это упрощает повторное использование UI и исключает их
случайное использование как индексов доменных данных.

## Проверенная матрица

| Область | Сценарии |
|---|---|
| Targets/auth | Pinned context/namespace/source chain; target/auth override rejection; container argv после `--`; global current-context не меняется |
| Sources | Независимые файлы с одинаковыми именами; conflicting context/server fail closed; directory-valued config; read-only inputs; saved profiles |
| Setup/import | Approval required; Esc/back не пишет; repeat setup; namespace не выдумывается; credential-safe metadata/errors; atomic replacement per file |
| Selectors | Поиск, Space/Tab, Ctrl+A, hidden selections, очистка поиска, Ctrl+O, draft/back, wide/narrow экраны |
| Logs | Single/multi, JSON full/compact, pause/details, bracketed paste, high-volume retention/render, literal search, previous logs, long matching lines |
| Pipe/save | Raw file permissions, plain stdout, separate multi stderr, request failure status, matching final line без newline |
| Exec | Native target/mode/workdir; piped dedent; command words/args; cancellation while stdin остаётся открытым |
| Scale | Native picker/input и pinned argv; invalid counts do not mutate |
| Jobs/CronJobs | Newest-first jobs; save-all-container logs; active scalar/array; startup-only retries; create failure cleanup |
| Restart | Supported controller/UID/selector guard; sequential replacement readiness; unsupported owners not deleted |
| Dashboards | Pinned read-only requests; single snapshots in non-TTY; key cancellation; incremental painter; same native runtime dispatch |
| Prod | Mutating workflows and plain kubectl commands fail closed without explicit confirmation |
| Distribution | Standalone help/config/preview/version без repo/Go/gum; command installation/collisions; four release targets |

Native exec/scale/jobs дополнительно пройдены через PTY. Старые gum fallback и fake-picker тесты удалены; общий UI теперь в `internal/tui`.
Старый источник ложной уверенности исправлен: namespace fixtures теперь имеют
**разные ответы для разных источников**. Проверка переключения смотрит свежий
screen после действия и фактический KUBECONFIG, а не старый текст в capture.

## Результат проверок

В этой рабочей копии после исправлений прошёл полный `make test`:

- runner contract;
- pytest-safe contract;
- `tools/dots` race и vet;
- helper workspace race и vet;
- Node/Pi extensions.

Failed suites: **0**. Дополнительно targeted CLI/PTY regressions проходились
отдельно в red → green циклах. Повторить:

```sh
go -C tools/dots test -race ./internal/kube/... ./cmd/dots-kube -count=1 -timeout=90s
go -C tools/dots vet ./internal/kube/... ./cmd/dots-kube
make test
./scripts/build-go-tools.sh
./scripts/build-kube-release.sh
```

Основные regression seams:
`tools/dots/cmd/dots-kube/regressions_test.go`, `namespace_test.go`,
`single_logs_test.go`, `logs_test.go`; `tools/dots/internal/kube/audit_test.go`.

## Ограничения и последующая проверка

- Профиль, **ранее ошибочно сохранённый** на чужой файл, не исправляется по имени
  окружения или файла: это было бы опасным угадыванием. Проверить `kpstage config`
  и явно выбрать верный файл/context через `kpstage setup`.
- Старые settings без нового поля context читаются; если несколько context в одном
  файле имеют одинаковые ссылки, прежний исходный выбор восстановить достоверно
  нельзя. Один раз выбрать его явно; новая запись запомнит выбор.
- Тесты не доказывают работоспособность конкретного RBAC, сети, auth plugin,
  прокси, Kubernetes/metrics-server версии или shell внутри реального контейнера.
  Namespace lookup может законно получить Forbidden; manual input сохраняется.
- Поиск логов/retention — по удерживаемым данным. По умолчанию 5000 записей/32 MiB;
  новые вытесняют старые. Display record ограничен 1 MiB, scanner — 16 MiB.
  Если запись не помещается по высоте, виден marker и доступен detail scroll.
  Для исходного полного вывода за пределами UI использовать logs-save/plain stream.
- Очень маленький терминал не может показывать все подсказки/поля одновременно;
  длинный Intro у Input и произвольные dashboards не имеют общего scrolling layout.
- Сохранение settings/overlays атомарно **по файлу**, не транзакционно по всему набору.
- CSV history pod analytics и process sampling — best-effort; конкурентные viewers
  одной history series и все особенности `/proc` здесь не проверены отдельно.
- macOS PTY проверен напрямую; Linux cross-build не заменяет интерактивную проверку
  на каждой Linux terminal implementation. Windows backend отсутствует.

Архитектура и правила переноса на новые тулы:
[Native Go UI](go-terminal-ui.md).
