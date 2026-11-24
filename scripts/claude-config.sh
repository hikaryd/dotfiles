#!/usr/bin/env bash
# Apply the shared Claude config from dots to ~/.claude and the claude-guard profile.
set -euo pipefail
umask 077

if [[ $# -ne 0 ]]; then
  printf 'Использование: scripts/claude-config.sh\n' >&2
  exit 2
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export CLAUDE_CONFIG_REPO="$REPO_DIR"

python3 -I - <<'PY'
import json
import os
import pathlib
import stat
import sys
import tempfile


def read_object(path):
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise RuntimeError(f"Expected JSON object in {path}")
    return data


def substitute(value, home):
    if isinstance(value, str):
        return value.replace("@HOME@", home)
    if isinstance(value, list):
        return [substitute(item, home) for item in value]
    if isinstance(value, dict):
        return {key: substitute(item, home) for key, item in value.items()}
    return value


def merge(target, source):
    for key, value in source.items():
        if isinstance(value, dict) and isinstance(target.get(key), dict):
            merge(target[key], value)
        else:
            target[key] = value


def update_json(path, change):
    # claude-guard rejects a symlinked or hard-linked settings.json, so merge in place instead of linking.
    if path.is_symlink():
        raise RuntimeError(f"Refusing symlink file: {path}")
    current = read_object(path) if path.exists() else {}
    updated = json.loads(json.dumps(current))
    change(updated)
    if updated == current:
        return False
    descriptor, name = tempfile.mkstemp(prefix=".dots-claude-", dir=path.parent)
    temporary = pathlib.Path(name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            stream.write(json.dumps(updated, indent=2, ensure_ascii=False) + "\n")
        temporary.chmod(0o600)
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)
    return True


def link(target, source, replace_identical=False):
    if target.is_symlink() and os.readlink(target) == str(source):
        return False
    if target.exists():
        if not replace_identical:
            return False
        if not target.is_file() or target.read_bytes() != source.read_bytes():
            raise RuntimeError(f"Refusing to replace a different local file: {target}")
        target.unlink()
    elif target.is_symlink():
        if not replace_identical:
            return False
        target.unlink()
    target.symlink_to(source)
    return True


def directory(path):
    path.mkdir(parents=True, exist_ok=True)
    if path.is_symlink() or not stat.S_ISDIR(path.lstat().st_mode):
        raise RuntimeError(f"Unsafe directory: {path}")
    return path


def main():
    repo = pathlib.Path(os.environ["CLAUDE_CONFIG_REPO"]) / "config/claude"
    home = pathlib.Path.home()
    profile = home / ".local/share/claude-guard/home/.claude"
    if profile.is_symlink() or not profile.is_dir():
        raise RuntimeError("Профиль claude-guard не найден. Сначала выполните tools/claude-setup/install.sh")

    changes = []
    settings = read_object(repo / "settings.json")
    if update_json(profile / "settings.json", lambda data: merge(data, settings)):
        changes.append("settings.json")

    servers = {}
    for name in ("mcp.json", "mcp.private.json"):
        path = repo / name
        if path.exists():
            found = read_object(path).get("mcpServers")
            if not isinstance(found, dict):
                raise RuntimeError(f"Expected mcpServers object in {path}")
            servers.update(substitute(found, str(home)))
    if update_json(profile / ".claude.json", lambda data: data.setdefault("mcpServers", {}).update(servers)):
        changes.append("MCP")

    # Under the guard ~/.claude/rules is only a project-level directory, where links leaving the project are skipped.
    for kind in ("rules", "agents"):
        destination = directory(profile / kind)
        for source in sorted((repo / kind).glob("*.md")):
            if link(destination / source.name, source, replace_identical=True):
                changes.append(f"{kind}/{source.name}")

    # Chisle ships its own skills via the Claude plugin; linking them too duplicates them.
    plugin_skills = {"chisle", "chisle-audit", "chisle-help", "chisle-review"}
    skills = directory(profile / "skills")
    managed_bases = tuple(pathlib.Path(os.path.normpath(b)) for b in (home / ".agents/skills", home / ".jcode/skills"))
    for entry in sorted(skills.iterdir()):
        if not entry.is_symlink():
            continue
        target = pathlib.Path(os.path.normpath(os.path.join(entry.parent, os.readlink(entry))))
        if target.parent not in managed_bases:
            continue
        if entry.name in plugin_skills or not (target / "SKILL.md").is_file():
            entry.unlink()
            changes.append(f"removed skills/{entry.name}")
    for base in managed_bases:
        if not base.is_dir():
            continue
        for source in sorted(base.iterdir()):
            if source.name.startswith(".") or source.name in plugin_skills or not (source / "SKILL.md").is_file():
                continue
            if link(skills / source.name, source):
                changes.append(f"skills/{source.name}")

    print("Конфигурация Claude: " + (", ".join(changes) if changes else "без изменений"))


try:
    main()
except (OSError, RuntimeError, ValueError) as error:
    print("claude config: " + str(error), file=sys.stderr)
    sys.exit(1)
PY
