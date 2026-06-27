#!/usr/bin/env bash
#
# Применяет версионированный конфиг Claude Code из config/claude/ к ~/.claude.
# Идемпотентно: бэкапит текущие settings.json / .omc-config.json, копирует
# снапшоты с подстановкой секретов и (пере)регистрирует MCP-серверы.
#
# Секреты берутся из ~/.claude/.mcp-secrets (вне репозитория). Серверы и env,
# которым не хватает секрета, аккуратно пропускаются с предупреждением.
#
# Запуск: scripts/claude-apply.sh  (вызывается из steps/claude.yml)

set -euo pipefail

CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$REPO_DIR/config/claude"
TS="$(date +%Y%m%d%H%M%S)"

if [ ! -d "$SRC" ]; then
  echo "claude-apply: каталог $SRC не найден" >&2
  exit 1
fi

mkdir -p "$CLAUDE_DIR"

echo "claude-apply: применяю конфиг в $CLAUDE_DIR"

# --- бэкап изменяемых файлов ---
for f in settings.json .omc-config.json; do
  if [ -f "$CLAUDE_DIR/$f" ]; then
    cp "$CLAUDE_DIR/$f" "$CLAUDE_DIR/$f.bak-$TS"
    echo "  backup: $f -> $f.bak-$TS"
  fi
done

# --- секреты ---
SECRETS="$CLAUDE_DIR/.mcp-secrets"
if [ -f "$SECRETS" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$SECRETS"
  set +a
  echo "  секреты загружены из $SECRETS"
else
  echo "  ⚠ $SECRETS не найден — env/MCP, требующие секретов, будут пропущены."
  echo "    cp config/claude/mcp-secrets.example.sh ~/.claude/.mcp-secrets && \$EDITOR ~/.claude/.mcp-secrets"
fi

HAVE_CLAUDE=0
command -v claude >/dev/null 2>&1 && HAVE_CLAUDE=1

export CLAUDE_DIR SRC HAVE_CLAUDE

python3 <<'PY'
import json, os, re, subprocess, shutil, sys

claude_dir = os.environ["CLAUDE_DIR"]
src = os.environ["SRC"]
have_claude = os.environ.get("HAVE_CLAUDE") == "1"

# whitelist переменных, которые разрешено подставлять в плейсхолдеры ${VAR}
WHITELIST = ("ANTHROPIC_BASE_URL", "Z_AI_API_KEY", "MAGIC_API_KEY", "KUBECONFIG_PATH")
PLACEHOLDER = re.compile(r"\$\{(\w+)\}")

def subst(value):
    """Подставляет ${VAR} из окружения (только whitelist, непустые значения)."""
    def repl(m):
        var = m.group(1)
        if var in WHITELIST and os.environ.get(var):
            return os.environ[var]
        return m.group(0)  # оставить плейсхолдер как есть
    if isinstance(value, str):
        return PLACEHOLDER.sub(repl, value)
    if isinstance(value, list):
        return [subst(v) for v in value]
    if isinstance(value, dict):
        return {k: subst(v) for k, v in value.items()}
    return value

def has_unresolved(obj):
    return bool(PLACEHOLDER.search(json.dumps(obj)))

# --- 1. omc-config.json (просто копия) ---
shutil.copyfile(f"{src}/omc-config.json", f"{claude_dir}/.omc-config.json")
print("  .omc-config.json: записан")

# --- 2. settings.json (подстановка ANTHROPIC_BASE_URL или удаление ключа) ---
with open(f"{src}/settings.json") as f:
    settings = json.load(f)

env = settings.get("env", {})
if "ANTHROPIC_BASE_URL" in env:
    if os.environ.get("ANTHROPIC_BASE_URL"):
        env["ANTHROPIC_BASE_URL"] = os.environ["ANTHROPIC_BASE_URL"]
        print("  settings.json: ANTHROPIC_BASE_URL подставлен из секрета")
    else:
        del env["ANTHROPIC_BASE_URL"]
        print("  settings.json: ANTHROPIC_BASE_URL не задан — ключ убран (дефолтный endpoint)")

with open(f"{claude_dir}/settings.json", "w") as f:
    json.dump(settings, f, indent=2, ensure_ascii=False)
    f.write("\n")
print("  settings.json: записан")

# --- 3. MCP-серверы ---
with open(f"{src}/mcp-servers.json") as f:
    servers = json.load(f)

if not have_claude:
    print("  ⚠ команда 'claude' не найдена в PATH — регистрация MCP пропущена")
    sys.exit(0)

registered, skipped = [], []
for name, cfg in servers.items():
    resolved = subst(cfg)
    if has_unresolved(resolved):
        missing = sorted(set(PLACEHOLDER.findall(json.dumps(resolved))))
        skipped.append((name, missing))
        continue
    subprocess.run(["claude", "mcp", "remove", name, "-s", "user"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    r = subprocess.run(["claude", "mcp", "add-json", name, json.dumps(resolved), "-s", "user"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
    if r.returncode == 0:
        registered.append(name)
    else:
        skipped.append((name, [f"ошибка: {r.stderr.strip()[:120]}"]))

if registered:
    print("  MCP зарегистрированы:", ", ".join(registered))
for name, why in skipped:
    print(f"  MCP пропущен: {name} (нет {', '.join(why)})")
PY

echo "claude-apply: готово"
