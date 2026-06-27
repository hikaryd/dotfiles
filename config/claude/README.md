# Claude Code config

Личная конфигурация [Claude Code](https://claude.com/claude-code) (OMC + MCP), versioned.

Здесь хранится **только рукотворное**, что не восстанавливается автоматически.
Инфраструктура OMC (хуки `scripts/hooks/`, `hud/`, `skills/`, `rules/`, `commands/`,
плагины) ставится через `omc setup` — её тут нет намеренно.

## Что внутри

| Файл | Куда применяется | Примечание |
|------|------------------|------------|
| `settings.json` | `~/.claude/settings.json` | Снапшот. Без project-specific permissions; пути приведены к `$HOME`; `ANTHROPIC_BASE_URL` вынесен в плейсхолдер |
| `omc-config.json` | `~/.claude/.omc-config.json` | Снапшот конфига OMC |
| `mcp-servers.json` | `~/.claude.json` → `mcpServers` (scope `user`) | Шаблон; секреты заменены на `${...}` |
| `mcp-secrets.example.sh` | образец для `~/.claude/.mcp-secrets` | Реальные ключи живут вне репозитория |

Секреты (`Z_AI_API_KEY`, `MAGIC_API_KEY`, `KUBECONFIG_PATH`, `ANTHROPIC_BASE_URL`)
**никогда не попадают в git** — только в `~/.claude/.mcp-secrets` (gitignored).

## Восстановление на новой машине

```bash
# 1. Инфраструктура Claude Code + OMC (хуки, hud, skills, плагины)
omc setup

# 2. Секреты (один раз)
cp ~/dots/config/claude/mcp-secrets.example.sh ~/.claude/.mcp-secrets
$EDITOR ~/.claude/.mcp-secrets     # вписать реальные значения

# 3. Применить личный конфиг + зарегистрировать MCP
cd ~/dots && ./install -c steps/claude.yml
```

Шаг идемпотентен: бэкапит текущие `settings.json` / `.omc-config.json` в `*.bak-<ts>`,
копирует снапшоты с подстановкой секретов и (пере)регистрирует MCP-серверы.
Без `~/.claude/.mcp-secrets` MCP/env-подстановка пропускаются с предупреждением.

## Обновление дотов после ручной донастройки Claude

После того как что-то поднастроили в Claude (новые permissions, MCP, env) — выгрузите
обратно в репозиторий безопасным скриптом (он вычищает секреты и project-specific записи):

```bash
~/dots/scripts/claude-sync.sh
git -C ~/dots add -p config/claude/    # просмотреть и закоммитить
```

## Что НЕ версионируется

- `~/.claude.json` целиком — runtime-стейт, `userID`, история проектов (берётся только `mcpServers`).
- `scripts/hooks/`, `hud/`, `skills/`, `rules/`, `commands/`, `agents/`, `statusline/` — OMC/плагины.
- `CLAUDE.md` / `AGENTS.md` — частично управляются OMC.
- Project-level MCP (`context7`, `exa`, `dual-graph`) — специфика конкретных проектов.
- Логи, кэши, `projects/`, `telemetry/`, `tasks/`, `session-*` — runtime.
