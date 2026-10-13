# Управление macOS через MCP: Peekaboo вместо Orca

## Выбор

Для локального рабочего стола в Pi и Codex выбран **Peekaboo 4.9.2**.
Это macOS CLI на Swift с готовым stdio MCP: скриншоты, Accessibility, клики,
ввод, клавиши, окна и меню. Установка через официальный `openclaw/tap`;
требуется macOS 15+. Обычные desktop-инструменты не требуют отдельного
API-ключа: рассуждение и vision выполняет уже запущенный Pi/Codex.
Источники: [README](https://github.com/openclaw/Peekaboo),
[MCP](https://github.com/openclaw/Peekaboo/blob/v4.9.2/docs/MCP.md),
[релиз](https://github.com/openclaw/Peekaboo/releases/tag/v4.9.2),
[формула Homebrew](https://github.com/openclaw/homebrew-tap/blob/main/Formula/peekaboo.rb).

Рассмотренные варианты:

| Вариант | Почему не выбран |
| --- | --- |
| [macOS-use](https://github.com/browser-use/macOS-use) | В README установка Python/uv и собственного агента с ключом провайдера; для существующих Pi/Codex удобнее готовый MCP с отдельными desktop-инструментами. |
| [mcp-remote-macos-use](https://github.com/baryhuang/mcp-remote-macos-use) | MCP через Docker и Screen Sharing/VNC с учётными данными macOS; для локального Mac это лишняя инфраструктура и доступ к удалённому экрану. |

## Переносимая настройка

- `Brewfile`: `openclaw/tap/peekaboo`.
- `config/pi/mcp.json`: шаблон только локального desktop-сервера. `pi-apply`
  сохраняет остальные пользовательские серверы и секреты, заменяет управляемую
  запись Peekaboo целиком и делает backup изменённого файла. Шаблон не является
  симлинком на пользовательский `mcp.json`.
- `config/codex/config.toml`: сервер `peekaboo`; стандартный `codex-apply`
  объединяет его с личной конфигурацией.
- `config/peekaboo/skill/SKILL.md`: общий skill Pi/Codex, установленный через
  `steps/tools.yml` в `~/.agents/skills/peekaboo`.

Оба клиента запускают `/opt/homebrew/bin/peekaboo mcp --allow-foreground`.
Foreground разрешён для полноценного управления, но конкретные операции
по умолчанию используют background-доставку. `agent`, `analyze` и `browser`
скрыты в Pi и исключены в Codex: задача этой интеграции — только рабочий стол,
не второй AI-агент и не замена browser MCP.

```sh
brew install openclaw/tap/peekaboo
./install -c steps/tools.yml  # в том числе общий skill
./install -c steps/pi.yml
./install -c steps/codex.yml
peekaboo permissions status
pi mcp list
codex mcp get peekaboo
```

В открытом Pi выполнить `/reload`; Codex перезапустить. Если macOS запрашивает
разрешения, человек выдаёт Screen Recording, Accessibility и, для синтетического
ввода, Event Synthesizing. [Официальная инструкция](https://github.com/openclaw/Peekaboo/blob/v4.9.2/docs/permissions.md).

## Проверка на этом Mac

При миграции проверены:

- версия 4.9.2 и наличие всех трёх разрешений;
- два одновременно работающих stdio MCP-процесса, handshake и каталог tools;
- inline-скриншот и свежие AX-элементы временного документа TextEdit;
- ввод латиницы и кириллицы, подтверждённый свежим чтением вторым MCP-клиентом;
- `Cmd+S`, подтверждённый содержимым временного файла;
- клик по кнопке закрытия, подтверждённый исчезновением только тестового окна;
- настоящий вызов MCP `permissions` из **Pi** и из **Codex**;
- тесты применения Pi: сохранение личных MCP-серверов, backup, идемпотентность,
  права файлов и отказ от изменения конфигов при некорректном JSON.

У Peekaboo доставленная, но не подтверждённая операция может возвращать
`isError:true` с `dispatched_unverified`. Это не доказательство, что действие
не выполнено. Общий skill требует новый снимок/readback перед следующим
действием или повтором; эта особенность проявилась и была проверена в smoke test.

Orca удаляется через `brew uninstall --cask stablyai/orca/orca`, **без `--zap`**:
пользовательские данные и история остаются для восстановления. Orca-хуки
агентов снимаются отдельно с backup и сохранением остальных хуков.
