#!/usr/bin/env python3
"""Установить закреплённый Abide и объединить его глобальные настройки."""

import json
import os
from pathlib import Path
import re
import runpy
import select
import shlex
import shutil
import subprocess
import tempfile
import time
import tomllib

ROOT = Path(__file__).resolve().parents[1]
HOME_DIR = Path.home()
VERSION = (ROOT / "config/abide/version").read_text().strip()
PREFIX = HOME_DIR / ".local/share/dots/abide" / VERSION
PACKAGE = PREFIX / "node_modules/@coldtea/abide"
WRAPPER = ROOT / "scripts/abide"
MARKER = "abide-hook.js"


def write(path, content, mode=None):
    """Сохранить предыдущую версию вне Git, заменить только изменённый файл."""
    if path.is_symlink():
        raise ValueError(f"ожидался обычный файл: {path}")
    original = path.read_bytes() if path.exists() else None
    encoded = content.encode()
    if original == encoded:
        if mode is not None:
            path.chmod(mode)
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    if original is not None:
        backup = HOME_DIR / ".config/dots-backups/abide" / str(time.time_ns())
        backup.mkdir(mode=0o700, parents=True)
        backup_file = backup / path.name
        shutil.copy2(path, backup_file)
        backup_file.chmod(0o600)
    fd, temporary = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.")
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(encoded)
        os.chmod(temporary, mode if mode is not None else (path.stat().st_mode & 0o777 if path.exists() else 0o600))
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def read_keys(path):
    if not path.exists():
        return {}
    found = {}
    for line in path.read_text().splitlines():
        match = re.match(r"^(?:export\s+)?([A-Z_][A-Z0-9_]*)\s*=\s*(.*)$", line.strip())
        if match:
            value = match[2].strip()
            if len(value) > 1 and value[0] == value[-1] and value[0] in "\"'":
                value = value[1:-1]
            if value:
                found[match[1]] = value
    return found


def configure_credentials():
    target = HOME_DIR / ".abide/.env"
    if target.is_symlink():
        raise ValueError(f"ожидался обычный файл ключей: {target}")
    keys = read_keys(target)
    names = ("TYPESAFE_AI_API_KEY", "OPENROUTER_API_KEY", "AI_GATEWAY_API_KEY")
    available = {name: os.environ.get(name) or keys.get(name) for name in names}
    # Настроенный TypeSafe/Gateway сохраняет приоритет; ключ OpenRouter идёт в его System One API.
    direct = available.get("TYPESAFE_AI_API_KEY") or ""
    gateway = available.get("AI_GATEWAY_API_KEY") or ""
    if (direct and not direct.startswith("sk-or-")) or (not direct and gateway and not gateway.startswith("sk-or-") and not available.get("OPENROUTER_API_KEY")):
        if target.exists():
            target.chmod(0o600)
        print("Abide: существующие ключи сохранены")
        return
    existing = read_keys(HOME_DIR / ".config/jevgrep/secrets.env") if not any(available.values()) else {}
    router = available.get("OPENROUTER_API_KEY") or next((value for value in (direct, gateway) if value.startswith("sk-or-")), None)
    router = router or existing.get("OPENROUTER_API_KEY")
    if not router and existing.get("AI_GATEWAY_API_KEY", "").startswith("sk-or-"):
        router = existing["AI_GATEWAY_API_KEY"]
    content = target.read_text() if target.exists() else ""
    if router:
        # Убираем только тот же OpenRouter key, ошибочно сохранённый под чужим именем.
        for name in ("AI_GATEWAY_API_KEY", "TYPESAFE_AI_API_KEY"):
            if keys.get(name) == router:
                content = re.sub(r"(?m)^(?:export\s+)?" + name + r"\s*=.*\n?", "", content)
        # URL назначает адаптация ключа: глобальный override мог бы перенаправить чужой project TypeSafe key.
        if keys.get("TYPESAFE_AI_BASE_URL") == "https://openrouter.ai/api/v1":
            content = re.sub(r"(?m)^(?:export\s+)?TYPESAFE_AI_BASE_URL\s*=.*\n?", "", content)
        for name, value in (("OPENROUTER_API_KEY", router),):
            pattern = r"(?m)^(?:export\s+)?" + name + r"\s*=.*$"
            line = f"{name}={value}"
            content = re.sub(pattern, lambda _: line, content) if re.search(pattern, content) else content.rstrip() + "\n" + line + "\n"
        write(target, content, 0o600)
        target.parent.chmod(0o700)
        print("Abide: OpenRouter → Jev System One API; ключ сохранён вне Git, значения не выводятся")
    elif existing.get("AI_GATEWAY_API_KEY"):
        write(target, content.rstrip() + f"\nAI_GATEWAY_API_KEY={existing['AI_GATEWAY_API_KEY']}\n", 0o600)
        target.parent.chmod(0o700)
        print("Abide: сохранён существующий Gateway key; значения не выводятся")
    else:
        print("Abide: задайте OPENROUTER_API_KEY в ~/.abide/.env или выполните abide login с TypeSafe/Gateway key")


def configure_hooks(target):
    data = json.loads(target.read_text()) if target.exists() else {}
    hooks = data.setdefault("hooks", {})
    for event, name, timeout in (
        ("SessionStart", "session-start", 10),
        ("UserPromptSubmit", "turn-start", 10),
        ("PostToolUse", "post-tool-use", 20),
        ("Stop", "stop", 30),
    ):
        kept = []
        for group in hooks.get(event, []):
            entries = [entry for entry in group["hooks"] if MARKER not in entry.get("command", "")]
            if entries:
                kept.append({**group, "hooks": entries})
        group = {"hooks": [{"type": "command", "command": f"{shlex.quote(str(WRAPPER))} --hook {name} # {MARKER}", "timeout": timeout}]}
        if event == "PostToolUse":
            group["matcher"] = "Edit|Write|MultiEdit|apply_patch"
        hooks[event] = kept + [group]
    write(target, json.dumps(data, indent=2, ensure_ascii=False) + "\n")
    print(f"Abide: четыре хука установлены в {target}")


def configure_instructions(target):
    begin, end = "<!-- BEGIN DOTS ABIDE -->", "<!-- END DOTS ABIDE -->"
    original = target.read_text() if target.exists() else ""
    block = begin + "\n" + (ROOT / "config/abide/instructions.md").read_text().rstrip() + "\n" + end
    if begin in original:
        before, rest = original.split(begin, 1)
        _, after = rest.split(end, 1)
        updated = before + block + after
    else:
        updated = original.rstrip() + "\n\n" + block + "\n"
    write(target, updated)


def list_codex_hooks(binary, directory=None):
    env = dict(os.environ)
    if directory is not None:
        env["CODEX_HOME"] = str(directory)
    process = subprocess.Popen([binary, "app-server"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, env=env)
    def send(payload):
        process.stdin.write((json.dumps(payload) + "\n").encode())
        process.stdin.flush()

    def response(request_id):
        deadline = time.monotonic() + 20
        pending = b""
        while time.monotonic() < deadline:
            ready, _, _ = select.select([process.stdout], [], [], max(0, deadline - time.monotonic()))
            if not ready:
                break
            chunk = os.read(process.stdout.fileno(), 65536)
            if not chunk:
                break
            pending += chunk
            while b"\n" in pending:
                line, pending = pending.split(b"\n", 1)
                payload = json.loads(line)
                if payload.get("id") == request_id:
                    if "error" in payload:
                        raise ValueError("Codex отклонил запрос hooks/list")
                    return payload["result"]
        raise ValueError("Codex не вернул hooks/list за отведённое время")
    try:
        send({"id": 1, "method": "initialize", "params": {"clientInfo": {"name": "dots_abide", "version": "1"}, "capabilities": {"experimentalApi": True}}})
        response(1)
        send({"method": "initialized", "params": {}})
        send({"id": 2, "method": "hooks/list", "params": {"cwds": [str(ROOT)]}})
        data = response(2)["data"]
        if len(data) != 1 or data[0]["errors"]:
            raise ValueError("Codex сообщил ошибки конфигурации хуков")
        return data[0]["hooks"]
    finally:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        process.stdin.close()
        process.stdout.close()


def replace_table(text, name, values):
    header = f"[{name}]"
    pattern = re.compile(r"(?ms)^" + re.escape(header) + r"[^\n]*\n(.*?)(?=^\[|\Z)")
    match = pattern.search(text)
    body = match[1] if match else ""
    for key, value in values.items():
        line = f"{key} = {json.dumps(value)}"
        if re.search(r"(?m)^\s*" + re.escape(key) + r"\s*=", body):
            body = re.sub(r"(?m)^\s*" + re.escape(key) + r"\s*=.*$", lambda _: line, body)
        else:
            body = body.rstrip() + "\n" + line + "\n"
    block = header + "\n" + body.lstrip("\n").rstrip() + "\n\n"
    return text[:match.start()] + block + text[match.end():] if match else text.rstrip() + "\n\n" + block


def configure_codex_trust(directory):
    binary = shutil.which("codex")
    if binary is None:
        print("Abide: Codex CLI не установлен; доверие к хукам настройте после установки через /hooks")
        return
    target = directory / "config.toml"
    original = target.read_text() if target.exists() else ""
    parsed = tomllib.loads(original)
    updated = original
    if parsed.get("features", {}).get("hooks") is not True:
        if "features" in parsed and "[features]" not in original:
            raise ValueError("нестандартная запись features Codex; сохранена без изменений")
        updated = replace_table(updated, "features", {"hooks": True})
        tomllib.loads(updated)
        if target.exists() and target.read_text() != original:
            raise ValueError("конфигурация Codex изменилась во время включения hooks")
        write(target, updated, 0o600)
    entries = list_codex_hooks(binary, directory)
    expected = {f"{shlex.quote(str(WRAPPER))} --hook {name} # {MARKER}" for name in ("session-start", "turn-start", "post-tool-use", "stop")}
    source = (directory / "hooks.json").resolve()
    ours = [entry for entry in entries if Path(entry["sourcePath"]).resolve() == source and entry.get("command") in expected]
    if len(ours) != 4 or {entry["command"] for entry in ours} != expected:
        raise ValueError("Codex не обнаружил четыре ожидаемых глобальных хука Abide")
    current = target.read_text()
    state = tomllib.loads(current).get("hooks", {}).get("state", {})
    updated = current
    for entry in ours:
        name = "hooks.state." + json.dumps(entry["key"])
        existing = state.get(entry["key"], {})
        if existing.get("trusted_hash") != entry["currentHash"] or existing.get("enabled") is False:
            if existing and f"[{name}]" not in current:
                raise ValueError("нестандартная запись доверия Abide; сохранена без изменений")
            updated = replace_table(updated, name, {"trusted_hash": entry["currentHash"], "enabled": True})
    tomllib.loads(updated)
    if target.read_text() != current:
        raise ValueError("конфигурация Codex изменилась во время настройки доверия")
    write(target, updated, 0o600)
    verified = list_codex_hooks(binary, directory)
    approved = [entry for entry in verified if entry.get("command") in expected and Path(entry["sourcePath"]).resolve() == source]
    if len(approved) != 4 or any(entry["trustStatus"] != "trusted" or not entry["enabled"] for entry in approved):
        raise ValueError("Codex не подтвердил доверие к четырём хукам Abide")
    before_other = {entry["key"]: (entry["currentHash"], entry["trustStatus"], entry["enabled"]) for entry in entries if entry not in ours}
    after_other = {entry["key"]: (entry["currentHash"], entry["trustStatus"], entry["enabled"]) for entry in verified if entry not in approved}
    if before_other != after_other:
        raise ValueError("состояние сторонних хуков изменилось; нужна проверка")
    print("Abide: Codex подтвердил четыре enabled/trusted хука; стороннее доверие сохранено")


def main():
    metadata = PACKAGE / "package.json"
    if not metadata.exists() or json.loads(metadata.read_text()).get("version") != VERSION:
        subprocess.run(["npm", "install", "--prefix", str(PREFIX), "--ignore-scripts", "--no-audit", "--no-fund", f"@coldtea/abide@{VERSION}"], check=True)
    patched = runpy.run_path(str(ROOT / "scripts/abide-runtime-patch.py"))["apply"](PACKAGE)
    print(f"Abide: совместимость OpenRouter и диагностика проверены; обновлено файлов: {len(patched)}")
    link = HOME_DIR / ".local/bin/abide"
    link.parent.mkdir(parents=True, exist_ok=True)
    if link.is_symlink() and link.resolve() == WRAPPER:
        pass
    elif link.exists() or link.is_symlink():
        raise ValueError(f"существующая команда не перезаписана: {link}")
    else:
        link.symlink_to(WRAPPER)
    configure_credentials()
    codex = Path(os.environ.get("CODEX_CONFIG_DIR") or os.environ.get("CODEX_HOME") or HOME_DIR / ".codex")
    claude = Path(os.environ.get("CLAUDE_CONFIG_DIR") or HOME_DIR / ".claude")
    configure_hooks(codex / "hooks.json")
    configure_hooks(claude / "settings.json")
    for directory, name in ((codex, "AGENTS.md"), (claude, "CLAUDE.md")):
        configure_instructions(directory / name)

    # OpenCode вызывает те же хуки через обёртку, включая proxy из GUI.
    plugin = (PACKAGE / "opencode/abide.mjs").read_text()
    needle = 'spawn("node", [HOOK, name],'
    if plugin.count(needle) != 1:
        raise ValueError("структура плагина OpenCode изменилась; проверьте адаптацию proxy")
    plugin = plugin.replace(needle, f'spawn({json.dumps(str(WRAPPER))}, ["--hook", name],')
    plugin = "// abide-opencode-plugin: managed by dots/scripts/abide-apply.py\n" + plugin
    plugin = re.sub(r'^const HOOK = .*\n', '', plugin, flags=re.M)
    write(HOME_DIR / ".config/opencode/plugins/abide.js", plugin)

    skill_source = (PACKAGE / "skills/abide-compile").resolve()
    for base in (HOME_DIR / ".agents/skills", codex / "skills", claude / "skills", HOME_DIR / ".config/opencode/skills"):
        target = base / "abide-compile"
        if target.is_symlink() and target.resolve() == skill_source:
            continue
        if target.is_dir() and not target.is_symlink() and (target / "SKILL.md").is_file() and (target / "SKILL.md").read_bytes() == (skill_source / "SKILL.md").read_bytes():
            continue
        if target.is_symlink() and target.resolve().is_relative_to((HOME_DIR / ".local/share/dots/abide").resolve()) and str(target.resolve()).endswith("/node_modules/@coldtea/abide/skills/abide-compile"):
            target.unlink()
        if target.exists() or target.is_symlink():
            raise ValueError(f"существующий skill не перезаписан: {target}")
        target.parent.mkdir(parents=True, exist_ok=True)
        target.symlink_to(skill_source)
    configure_codex_trust(codex)
    print(f"Abide {VERSION}: глобальная установка готова")


if __name__ == "__main__":
    main()
