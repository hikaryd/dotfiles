#!/usr/bin/env bash
# Собирает и устанавливает SpellcheckMac.app (RU автозамена слов + локальный SAGE-сервер предложений)
# и регистрирует LaunchAgent. Идемпотентен по source.sha256 (принудительно: --force).
set -euo pipefail

BASEDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOL_DIR="$BASEDIR/tools/spellcheck"
SERVER_DIR="$TOOL_DIR/server"
APP_NAME="SpellcheckMac"
strip_trailing_slashes() {
    local p="$1"
    while [[ "$p" != "/" && "$p" == */ ]]; do
        p="${p%/}"
    done
    printf '%s' "$p"
}

APPS_DIR="$(strip_trailing_slashes "${DOTS_APPLICATIONS_DIR:-/Applications}")"
APP_BUNDLE="$APPS_DIR/${APP_NAME}.app"
APP_EXEC="$APP_BUNDLE/Contents/MacOS/${APP_NAME}"
SCRATCH_DIR="$BASEDIR/build/spellcheck"
AGENT_LABEL="local.dots.spellcheck"
LAUNCH_AGENTS_DIR="$(strip_trailing_slashes "${DOTS_LAUNCH_AGENTS_DIR:-$HOME/Library/LaunchAgents}")"
AGENT_PLIST="$LAUNCH_AGENTS_DIR/${AGENT_LABEL}.plist"
OLD_AGENT_PLIST="$LAUNCH_AGENTS_DIR/local.spellcheckmac.app.plist"
STAMP_FILE="$APP_BUNDLE/Contents/Resources/source.sha256"
VENV_STAMP="$SERVER_DIR/.venv/.requirements.sha256"
LAYOUT=2

FORCE=0
[[ "${1:-}" == "--force" ]] && FORCE=1

for cmd in swift uv iconutil codesign python3; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "spellcheck-app: не найден $cmd, пропуск" >&2
        exit 0
    fi
done

if [[ -L "$APPS_DIR" || -L "$APP_BUNDLE" || -L "$LAUNCH_AGENTS_DIR" ]]; then
    echo "spellcheck-app: отказ работать с симлинком в целевом пути" >&2
    exit 1
fi

ensure_venv() {
    local req_sum
    req_sum="$(shasum -a 256 "$SERVER_DIR/requirements.txt" | awk '{print $1}')"
    if [[ -x "$SERVER_DIR/.venv/bin/python" && -f "$VENV_STAMP" && "$(<"$VENV_STAMP")" == "$req_sum" ]]; then
        return 0
    fi
    echo "spellcheck-app: настраиваю Python-окружение ($SERVER_DIR/.venv)..."
    (
        cd "$SERVER_DIR"
        if [[ -L ".venv" ]]; then
            echo "spellcheck-app: отказ работать с симлинком .venv" >&2
            exit 1
        fi
        if [[ ! -x ".venv/bin/python" ]]; then
            rm -rf .venv
            uv venv --python 3.12 .venv
        fi
        uv pip install --python .venv/bin/python -r requirements.txt
    )
    printf '%s\n' "$req_sum" >"$VENV_STAMP"
}

compute_source_sum() {
    (
        cd "$TOOL_DIR"
        find app/Package.swift app/Sources server/server.py server/requirements.txt make_icon.swift \
            -type f | LC_ALL=C sort | xargs shasum -a 256
    ) | shasum -a 256 | awk '{print $1}'
}

# Стабильная подпись нужна для TCC: ad-hoc привязывает Accessibility к cdhash одной сборки.
select_codesign_identity() {
    if [[ -n "${DOTS_CODESIGN_IDENTITY:-}" ]]; then
        printf '%s' "$DOTS_CODESIGN_IDENTITY"
        return 0
    fi
    if ! command -v security >/dev/null 2>&1; then
        printf '%s' "-"
        return 0
    fi
    local identities line hash name dev_id="" apple_dev="" mac_dev="" other=""
    local re='^[[:space:]]*[0-9]+\)[[:space:]]+([0-9A-Fa-f]{40})[[:space:]]+"(.+)"[[:space:]]*$'
    identities="$(security find-identity -v -p codesigning 2>/dev/null || true)"
    while IFS= read -r line; do
        if [[ "$line" =~ $re ]]; then
            hash="${BASH_REMATCH[1]}"
            name="${BASH_REMATCH[2]}"
            if [[ -z "$dev_id" && "$name" == "Developer ID Application:"* ]]; then
                dev_id="$hash"
            elif [[ -z "$apple_dev" && "$name" == "Apple Development:"* ]]; then
                apple_dev="$hash"
            elif [[ -z "$mac_dev" && "$name" == "Mac Development:"* ]]; then
                mac_dev="$hash"
            elif [[ -z "$other" && "$name" != *"Distribution"* && "$name" != *"Installer"* ]]; then
                other="$hash"
            fi
        fi
    done <<<"$identities"

    if [[ -n "$dev_id" ]]; then
        printf '%s' "$dev_id"
    elif [[ -n "$apple_dev" ]]; then
        printf '%s' "$apple_dev"
    elif [[ -n "$mac_dev" ]]; then
        printf '%s' "$mac_dev"
    elif [[ -n "$other" ]]; then
        printf '%s' "$other"
    else
        printf '%s' "-"
    fi
}

extract_designated_requirement() {
    codesign -d -r - "$1" 2>&1 | awk '!found && /^#?[[:space:]]*designated => / { sub(/^#?[[:space:]]*designated => /, ""); print; found=1 }'
}

# Ищет процессы текущего UID строго по argv (без pkill -f), чтобы не задеть редакторы или чужой DOTS_APPLICATIONS_DIR.
find_matching_pids() {
    local mode="$1" target="$2"
    python3 - "$mode" "$target" <<'PY'
import os, subprocess, sys

mode, target = sys.argv[1], sys.argv[2]
uid = str(os.getuid())
out = subprocess.check_output(
    ["ps", "-u", uid, "-o", "pid=,args="],
    encoding="utf-8",
    errors="replace",
)
self_pid = os.getpid()
parent_pid = os.getppid()

for line in out.splitlines():
    parts = line.strip().split(None, 1)
    if len(parts) < 2:
        continue
    pid = int(parts[0])
    if pid in (self_pid, parent_pid):
        continue
    args = parts[1]
    if mode == "app":
        if args == target or args.startswith(target + " "):
            print(pid)
    elif mode == "server":
        py_bin, script = target.split("\t", 1)
        prefix = f"{py_bin} {script}"
        if args == prefix or args.startswith(prefix + " "):
            print(pid)
PY
}

stop_exact_processes() {
    local mode="$1" target="$2" pids
    pids="$(find_matching_pids "$mode" "$target")"
    [[ -z "$pids" ]] && return 0

    while IFS= read -r pid; do
        [[ -n "$pid" ]] && kill -TERM "$pid" 2>/dev/null || true
    done <<<"$pids"

    for _ in $(seq 1 30); do
        if [[ -z "$(find_matching_pids "$mode" "$target")" ]]; then
            return 0
        fi
        sleep 0.1
    done

    pids="$(find_matching_pids "$mode" "$target")"
    while IFS= read -r pid; do
        [[ -n "$pid" ]] && kill -KILL "$pid" 2>/dev/null || true
    done <<<"$pids"
    sleep 0.2
}

install_launch_agent() {
    if [[ -f "$OLD_AGENT_PLIST" && ! -L "$OLD_AGENT_PLIST" ]]; then
        if [[ "$LAUNCH_AGENTS_DIR" == "$HOME/Library/LaunchAgents" ]]; then
            launchctl bootout "gui/$(id -u)/local.spellcheckmac.app" 2>/dev/null || true
        fi
        rm -f "$OLD_AGENT_PLIST"
    fi

    mkdir -p "$LAUNCH_AGENTS_DIR"
    SPELLCHECK_AGENT="$AGENT_PLIST" \
    SPELLCHECK_LABEL="$AGENT_LABEL" \
    SPELLCHECK_EXEC="$APP_EXEC" \
    python3 - <<'PY'
import os, plistlib, tempfile
from pathlib import Path

path = Path(os.environ["SPELLCHECK_AGENT"])
if path.parent.is_symlink() or path.is_symlink():
    raise SystemExit("spellcheck-app: refusing LaunchAgent symlink")

job = {
    "Label": os.environ["SPELLCHECK_LABEL"],
    "ProgramArguments": [os.environ["SPELLCHECK_EXEC"]],
    "RunAtLoad": True,
    "KeepAlive": False,
    "ProcessType": "Interactive",
    "LimitLoadToSessionType": "Aqua",
}
if path.exists():
    try:
        with path.open("rb") as f:
            if plistlib.load(f) == job:
                raise SystemExit(0)
    except SystemExit:
        raise
    except Exception:
        # Битый/пустой plist перезаписываем атомарно ниже.
        pass

# mkstemp использует O_CREAT|O_EXCL|O_NOFOLLOW — защита от симлинк-атаки на временный файл.
fd, tmp_name = tempfile.mkstemp(dir=str(path.parent), prefix=f".{path.name}.")
try:
    with os.fdopen(fd, "wb") as f:
        plistlib.dump(job, f)
    os.chmod(tmp_name, 0o644)
    os.replace(tmp_name, path)
except BaseException:
    try:
        os.unlink(tmp_name)
    except OSError:
        pass
    raise
PY
}

ensure_venv

SIGN_IDENTITY="$(select_codesign_identity)"
SRC_SUM="$(compute_source_sum)"
EXPECTED_STAMP="$SRC_SUM $TOOL_DIR $LAYOUT $SIGN_IDENTITY"

if [[ "$FORCE" -eq 0 && ! -L "$APP_BUNDLE" && -d "$APP_BUNDLE" && ! -L "$STAMP_FILE" && -f "$STAMP_FILE" && "$(<"$STAMP_FILE")" == "$EXPECTED_STAMP" ]]; then
    if [[ "$APPS_DIR" == "/Applications" || -n "${DOTS_LAUNCH_AGENTS_DIR:-}" ]]; then
        install_launch_agent
    fi
    if [[ "$APPS_DIR" == "/Applications" && -z "$(find_matching_pids app "$APP_EXEC")" ]]; then
        open "$APP_BUNDLE"
    fi
    echo "SpellcheckMac.app актуален"
    exit 0
fi

echo "spellcheck-app: сборка ${APP_NAME} (release)..."
mkdir -p "$SCRATCH_DIR" "$APPS_DIR"
(cd "$TOOL_DIR/app" && swift build -c release --scratch-path "$SCRATCH_DIR")
BIN_PATH="$SCRATCH_DIR/release/${APP_NAME}"

tmp="$(mktemp -d "$APPS_DIR/.${APP_NAME}.app.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/Contents/MacOS" "$tmp/Contents/Resources"
cp "$BIN_PATH" "$tmp/Contents/MacOS/${APP_NAME}"
printf '%s\n' "$TOOL_DIR" >"$tmp/Contents/Resources/project_root.txt"
printf '%s\n' "$EXPECTED_STAMP" >"$tmp/Contents/Resources/source.sha256"

iconset="$tmp/AppIcon.iconset"
swift "$TOOL_DIR/make_icon.swift" "$iconset" >/dev/null
iconutil -c icns "$iconset" -o "$tmp/Contents/Resources/AppIcon.icns"
rm -rf "$iconset"

cat >"$tmp/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDisplayName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleExecutable</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>${AGENT_LABEL}</string>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>0.2.0</string>
    <key>CFBundleVersion</key>
    <string>${LAYOUT}</string>
    <key>LSMinimumSystemVersion</key>
    <string>14.0</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

if [[ "$SIGN_IDENTITY" == "-" ]]; then
    echo "spellcheck-app: не найден сертификат подписи, использую ad-hoc (Accessibility может сбрасываться при пересборке)" >&2
fi
codesign --force --timestamp=none --sign "$SIGN_IDENTITY" "$tmp" >/dev/null
codesign --verify --deep --strict "$tmp" >/dev/null
chmod -R u=rwX,go=rX "$tmp"

# При смене Designated Requirement (например, ad-hoc -> сертификат) старый csreq в TCC.db
# не обновляется простым переключением тумблера в Настройках — сбрасываем запись один раз.
RESET_TCC=0
if [[ "$APPS_DIR" == "/Applications" && -d "$APP_BUNDLE" && ! -L "$APP_BUNDLE" ]]; then
    OLD_DR="$(extract_designated_requirement "$APP_BUNDLE" || true)"
    if [[ -n "$OLD_DR" ]] && ! codesign --verify --strict -R "=$OLD_DR" "$tmp" >/dev/null 2>&1; then
        RESET_TCC=1
    fi
fi

stop_exact_processes app "$APP_EXEC"
if [[ "$APPS_DIR" == "/Applications" ]]; then
    stop_exact_processes server "$SERVER_DIR/.venv/bin/python	$SERVER_DIR/server.py"
fi

rm -rf "$APP_BUNDLE"
mv "$tmp" "$APP_BUNDLE"
trap - EXIT

if [[ "$APPS_DIR" == "/Applications" ]]; then
    /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$APP_BUNDLE" >/dev/null 2>&1 || true
    if [[ "$RESET_TCC" -eq 1 && -n "$AGENT_LABEL" ]]; then
        if command -v tccutil >/dev/null 2>&1 && tccutil reset Accessibility "$AGENT_LABEL" >/dev/null; then
            echo "spellcheck-app: подпись изменилась, сбросил старую запись Accessibility для $AGENT_LABEL — выдай доступ один раз"
        else
            echo "spellcheck-app: подпись изменилась, но сбросить Accessibility автоматически не удалось — выполни: tccutil reset Accessibility $AGENT_LABEL (или удали '-' и добавь заново в Настройках)" >&2
        fi
    fi
fi
if [[ "$APPS_DIR" == "/Applications" || -n "${DOTS_LAUNCH_AGENTS_DIR:-}" ]]; then
    install_launch_agent
fi
if [[ "$APPS_DIR" == "/Applications" ]]; then
    open "$APP_BUNDLE"
fi

echo "SpellcheckMac.app установлен в $APPS_DIR (LaunchAgent: $AGENT_LABEL)"
