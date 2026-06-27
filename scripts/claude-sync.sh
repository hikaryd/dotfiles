#!/usr/bin/env bash
#
# Выгружает текущий конфиг Claude Code из ~/.claude обратно в config/claude/
# (для коммита в доты). Безопасно: вычищает секреты и машинно-/проектно-
# специфичные данные. Реальные ключи НИКОГДА не пишутся в репозиторий.
#
# Запуск: scripts/claude-sync.sh

set -euo pipefail

CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DST="$REPO_DIR/config/claude"

mkdir -p "$DST"
export CLAUDE_DIR DST HOME

python3 <<'PY'
import json, os, re, sys

claude_dir = os.environ["CLAUDE_DIR"]
dst = os.environ["DST"]
home = os.path.expanduser("~")

# (server, env-key) -> имя переменной-секрета (как в ~/.claude/.mcp-secrets)
SECRET_MAP = {
    ("zai-mcp-server", "Z_AI_API_KEY"): "Z_AI_API_KEY",
    ("magic", "API_KEY"): "MAGIC_API_KEY",
}
SECRET_KEY_RE = re.compile(r"(KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL)", re.I)
warnings = []

# --- 1. settings.json ---
with open(f"{claude_dir}/settings.json") as f:
    s = json.load(f)

allow = s.get("permissions", {}).get("allow", [])
s["permissions"]["allow"] = [a for a in allow if "/Users/" not in a]
if "env" in s and "ANTHROPIC_BASE_URL" in s["env"]:
    s["env"]["ANTHROPIC_BASE_URL"] = "${ANTHROPIC_BASE_URL}"

txt = json.dumps(s, indent=2, ensure_ascii=False)
txt = txt.replace(home, "$HOME")  # пути хуков -> переносимые
with open(f"{dst}/settings.json", "w") as f:
    f.write(txt + "\n")
print(f"  settings.json: allow {len(allow)} -> {len(s['permissions']['allow'])}, base_url -> плейсхолдер")

# --- 2. .omc-config.json ---
with open(f"{claude_dir}/.omc-config.json") as f:
    omc = json.load(f)
with open(f"{dst}/omc-config.json", "w") as f:
    json.dump(omc, f, indent=2, ensure_ascii=False)
    f.write("\n")
print("  omc-config.json: записан")

# --- 3. mcp-servers.json (из ~/.claude.json, секреты замаскированы) ---
with open(os.path.expanduser("~/.claude.json")) as f:
    cj = json.load(f)
ms = cj.get("mcpServers", {})

def mask(name, cfg):
    cfg = json.loads(json.dumps(cfg))
    for k, v in list(cfg.get("env", {}).items()):
        if (name, k) in SECRET_MAP:
            cfg["env"][k] = "${" + SECRET_MAP[(name, k)] + "}"
        elif SECRET_KEY_RE.search(k):
            cfg["env"][k] = "${" + k + "}"
            warnings.append(f"{name}.env.{k}: замаскирован как ${{{k}}} — добавьте его в ~/.claude/.mcp-secrets и в SECRET_MAP скриптов")
    # kubeconfig путь -> плейсхолдер
    if "args" in cfg:
        cfg["args"] = ["${KUBECONFIG_PATH}" if (isinstance(a, str) and a.endswith((".yaml", ".yml")) and "/Users/" in a) else a
                       for a in cfg["args"]]
    return cfg

out = {name: mask(name, cfg) for name, cfg in ms.items()}
with open(f"{dst}/mcp-servers.json", "w") as f:
    json.dump(out, f, indent=2, ensure_ascii=False)
    f.write("\n")
print("  mcp-servers.json: серверы ->", ", ".join(out.keys()))

for w in warnings:
    print("  ⚠", w)

# финальная страховка: значения секретных env-ключей должны быть плейсхолдерами ${...}.
# (Проверяем именно ЗНАЧЕНИЯ секретных ключей, а не любые длинные строки — иначе
#  ложно срабатывает на длинных именах настроек и permissions.)
def env_leaks(obj, label):
    out = []
    for k, v in (obj.get("env", {}) or {}).items():
        if isinstance(v, str) and SECRET_KEY_RE.search(k) and not v.startswith("${"):
            out.append(f"{label}.env.{k}")
    return out

leaks = env_leaks(s, "settings")
leaks += [l for name, cfg in out.items() for l in env_leaks(cfg, name)]
if leaks:
    print("  ✗ ВНИМАНИЕ: незамаскированные секреты в выгрузке:", leaks, file=sys.stderr)
    sys.exit(1)
print("  ✓ секреты замаскированы")
PY

echo ""
echo "Готово. Проверьте изменения перед коммитом:"
git -C "$REPO_DIR" status --short config/claude/
echo ""
echo "  git -C \"$REPO_DIR\" add -p config/claude/"
