# Аудит собственных скриптов перед переносом с Python

## Границы проверки

Проверены ссылки из `install`, `uninstall`, `steps/`, конфигов shell/Git/Herdr/
tmux, меню `dots`, документации и других собственных скриптов. Проверены имена и
назначение установленных ссылок в `~/.local/bin`, но не история команд,
credentials или содержимое рабочих сессий. Сторонний Dotbot, виртуальные
окружения, caches и приватные файлы не считаются собственными скриптами.

Снимок: **36 собственных Python entrypoints / около 11 тысяч строк** без тестов.
Включены extensionless `scripts/dots`, `scripts/dots-nu`, `scripts/ai_helper`.
Это не означает, что все они запускаются часто. Статический вызов доказывает
подключение, а не фактическую частоту. Отсутствие ссылки не доказывает отсутствие
ручного использования или внешнего automation.

## Подключённый код: сохранять поведение, а не удалять

| Группа | Скрипты | Подтверждённые вызывающие места |
|---|---|---|
| Главное меню и оболочка | `scripts/dots`, `scripts/dots-nu` | `steps/terminal.yml`, Ghostty/Herdr default shell, Nushell bridge |
| Повседневные действия | `dots-{projects,sessions,status,refs,clip,private,style,doctor,notify}.py` | Динамический dispatch в `scripts/dots`; часть команд вызывается хоткеями Herdr/tmux |
| Оформление и уведомления | `dots-{ghostty-reload,tmux-style,herdr-status}.py`, `dots-notify.py` | `dots-style.py`, Herdr tab bar, tmux config, zsh precmd hook |
| Названия задач агентов | `config/herdr/plugins/task-titles/titles.py`, `codex-task-title.py` | Манифест task-titles, Codex hook и интеграция Pi |
| AI для Git | `scripts/ai_helper` | `scripts/ai_commit`, `config/git/hooks/prepare-commit-msg`, LazyGit, `create_mr` alias |
| Прокси | `spotify-proxy.py`, `config/xray-codex/codex-proxy.py` | Spotify launcher, shell alias и ссылки установки |
| Установка AI-настроек | `codex_config.py`, `codex-instructions-apply.py`, `codex-task-title-apply.py`, `codex-bmad-apply.py`, `abide-apply.py`, `abide-runtime-patch.py`, `pi-apply.py` | Установка, sync/uninstall, wrappers и вызовы между helpers |
| macOS/browser | `look-hotkey.py`, `waterfox-apply.py` | `steps/macos.yml`, `steps/tools.yml` |
| Палитра | `palette/generate.py` | Полный `install`, выбор tool/variant вручную |
| VPN setup/runtime | `vpn/{generate,router-setup,tcp-tuning}.py` | `vpn/install.sh`, `vps-setup.sh`, генерируемые systemd units |
| VPN diagnostics | `vpn/{benchmark,xhttp-benchmark}.py` | Документированные ручные entrypoints в `vpn/README.md` |

### Ложные кандидаты на удаление

- `dots-clip.py`, `dots-refs.py`, `dots-sessions.py` и другие `dots-*` могут не
  иметь буквального упоминания своего filename: путь строится динамически.
  Удаление по результату простого `rg filename` сломает меню и хоткеи.
- `ai_helper` не является старой ненужной альтернативой coding agent:
  используется отдельно для commit/MR и Git hook. Сам hook не следует запускать
  для аудита, поскольку он может обращаться к провайдеру или менять commit message.
- VPN benchmarks — ручные диагностические инструменты, а не функции основного
  installer. Отсутствие automatic call не делает их неиспользуемыми.
- Палитра Thorium/Sketchybar и настройка Zen выглядят legacy относительно основной
  таблицы программ, но **ещё включены** в generator/steps. Отказ от этих приложений
  надо решить отдельно и убрать весь путь установки, а не только кусок генератора.

## Кандидаты, требующие решения владельца

| Кандидат | Что известно | Что не доказано |
|---|---|---|
| `scripts/migrate_claude_to_codex.py` | Разовая миграция; нет caller в текущей установке/конфигах/доках | Миграция завершена или этот инструмент больше не потребуется |
| `scripts/macos-defaults.sh` | Нет caller в текущей установке; часть настроек дублирует `steps/macos.yml`, `spans-displays=true` противоречит нынешнему `false` | Скрипт не запускают вручную ради других настроек |
| `vpn/pixel-proxy-adb.py` и `pixel-proxy-service.sh` | Отдельный ручной Android/Termux gateway; нет вызова из основного VPN installer | Нет внешнего запуска/boot automation или действующего использования |

Эти файлы не удалены: недостаточно evidence, чтобы считать их безопасно
неиспользуемыми. Для удаления нужно подтвердить отказ от соответствующей
возможности; связанные tests/config/docs удаляются вместе с ней, а не отдельно.

## Подтверждённая лишняя зависимость

Глобальная установка PyYAML через `pip3 --break-system-packages` в
`steps/dependencies.yml` не нужна: Dotbot уже подключает
`dotbot/lib/pyyaml/lib` в своём launcher, собственные production Python-модули
не импортируют `yaml`.

`uninstall` имел optional `import yaml`, поэтому его read-only inventory теперь
использует ту же bundled library. Минимальный fallback без Dotbot также сохраняет
unquoted пути с пробелами. Это проверено с `python -S` в временном fixture:
глобальные Python packages не могут скрыть ошибку. Bootstrap-шаг глобального pip
удалён; никакие уже установленные пакеты на машине не удалялись.

## Тесты: сначала единый запуск, затем перенос

Существующие наборы не содержат очевидных безопасно удаляемых дубликатов.
Базовый явный прогон: **78 Python tests, 20 Node tests**, Go last-workspace также
проходит с `-race`. Корневой `unittest discover` при этом запускал **0 tests**.
Это проблема обнаружения, а не доказательство отсутствия нужных тестов.
После добавления двух uninstall regression-тестов и параллельных обновлений
Herdr/Pi финальный прогон общей команды собрал **87 Python tests и 30 Node tests**;
Go race/vet и проверки самого runner также прошли. Количество — снимок,
актуальный состав всегда показывает `make test`.

Добавлены единый `make test` / `scripts/test.sh`, проверка отказа на пустых
Python/Node наборах и два regression-теста uninstall inventory без глобальных
packages. Наборы оставлены рядом с кодом; не вводится ещё один framework и не
переносятся параллельно редактируемые тесты ради косметики.

Оставить проверки:

- сохранения auth, пользовательских настроек, symlinks и backups;
- владения pane/tab metadata и защиты ручных названий;
- ownership маршрутов, rollback и сохранения чужих ресурсов;
- redaction credentials и неуспешных HTTP-ответов;
- реальной очистки собственного дочернего процесса и временных файлов;
- socket protocol, конкуренции, таймаутов и workspace history;
- честного отображения неизвестных quota/TPS вместо выдуманных значений.

Похожие проверки HTTP 200/полного download в sing-box и Xray не являются
взаимозаменяемыми: разные implementations и failure paths. Их можно делать
табличными внутри своего набора, но удаление одной лишит второй путь защиты.

**Пробелы:** у трёх Kubernetes Go watchers нет тестов; `codex_config.py` и большая
часть CLI `dots` не имеют characterization-наборов. Общий зелёный прогон их
behaviour не доказывает. Состав и правила запуска: [tests/README.md](../tests/README.md).

## Порядок следующего переноса

1. Частые runtime helpers: Herdr status/task titles, Codex title hook,
   notifications и shell launcher. Измерять целую команду, а не только её логику.
2. CLI `dots` и его повседневные действия. Предпочесть один Go binary с
   subcommands вместо отдельного framework и множества wrappers.
3. Установщики и `codex_config.py`: сначала fixtures сохранения пользовательских
   TOML/JSON и leak gate, затем native replacement. Не запускать реальные apply/
   sync/uninstall для подтверждения тестов.
4. Генератор палитры: golden fixtures для нужных tools/variants. Перед удалением
   legacy tools подтвердить, что их приложения действительно больше не нужны.
5. VPN: отдельный перенос с protocol/security/ownership tests и проверкой
   архитектуры Linux-бинарников. Изменение репозитория само по себе не означает
   migration запущенных служб на VPS/роутере/телефоне.

Удалять Python implementation и её tests только после проверки замены и всех
вызывающих мест. Не заменять их заглушкой или Go wrapper, который скрыто запускает
Python. Короткий Bash для orchestration допустим: это не runtime с логикой
каждого хоткея.
