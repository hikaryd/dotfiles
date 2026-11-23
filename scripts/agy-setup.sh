#!/usr/bin/env bash
# Assemble the local Antigravity plugin from shared dots templates and machine-local overlays.
set -euo pipefail
umask 077

if [[ $# -ne 0 ]]; then
  printf 'Использование: scripts/agy-setup.sh\n' >&2
  exit 2
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export AGY_SETUP_REPO="$REPO_DIR"

python3 -I - <<'PY'
import fcntl
import json
import os
import pathlib
import shutil
import stat
import subprocess
import sys
import tempfile

ALLOWED_MCP_KEYS = {"command", "args", "env", "cwd", "url", "headers", "disabled", "variables"}
SKIP_SKILLS = {"claude-statusbar"}
MAX_RULE_BYTES = 24000


def ensure_safe_dir(path, mode=0o755):
    try:
        path.mkdir(mode=mode, exist_ok=True)
    except FileExistsError:
        pass
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode):
        raise RuntimeError(f"Refusing symlink directory: {path}")
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid():
        raise RuntimeError(f"Unsafe directory: {path}")
    if info.st_mode & 0o022:
        raise RuntimeError(f"Directory is writable by others: {path}")


def read_private_json(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise RuntimeError(f"Unsafe JSON file: {path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise RuntimeError(f"Expected JSON object in {path}")
    return data


def substitute(value, replacements):
    if isinstance(value, str):
        for token, target in replacements.items():
            value = value.replace(token, target)
        return value
    if isinstance(value, list):
        return [substitute(item, replacements) for item in value]
    if isinstance(value, dict):
        return {k: substitute(v, replacements) for k, v in value.items()}
    return value


def clean_mcp_entry(cfg):
    if not isinstance(cfg, dict):
        return None
    cleaned = {k: v for k, v in cfg.items() if k in ALLOWED_MCP_KEYS}
    has_cmd = isinstance(cleaned.get("command"), str) and bool(cleaned["command"].strip())
    has_url = isinstance(cleaned.get("url"), str) and bool(cleaned["url"].strip())
    return cleaned if (has_cmd or has_url) else None


def build_mcp(repo, home, jcode_data):
    replacements = {"@HOME@": str(home), "@JCODE_DATA@": str(jcode_data)}
    servers = {}

    base_path = repo / "config/antigravity/mcp.json"
    base_data = json.loads(base_path.read_text(encoding="utf-8"))
    base_servers = base_data.get("mcpServers") if isinstance(base_data, dict) else None
    if not isinstance(base_servers, dict):
        raise RuntimeError(f"Expected mcpServers object in {base_path}")
    for name, cfg in base_servers.items():
        resolved = clean_mcp_entry(substitute(cfg, replacements))
        if not resolved:
            continue
        cmd = resolved.get("command")
        if cmd and os.path.isabs(cmd) and not os.access(cmd, os.X_OK):
            continue
        servers[name] = resolved

    # Legacy ~/.claude.json first so active claude-guard settings take precedence.
    for claude_json in (
        home / ".claude.json",
        home / ".local/share/claude-guard/home/.claude/.claude.json",
    ):
        if claude_json.exists() and not claude_json.is_symlink():
            cdata = read_private_json(claude_json)
            raw_servers = cdata.get("mcpServers")
            if isinstance(raw_servers, dict):
                for name, cfg in raw_servers.items():
                    cleaned = clean_mcp_entry(cfg)
                    if cleaned:
                        servers[name] = cleaned

    for overlay in (
        home / ".config/antigravity/mcp.private.json",
        repo / "config/antigravity/mcp.private.json",
    ):
        if overlay.exists() and not overlay.is_symlink():
            odata = read_private_json(overlay)
            raw_servers = odata["mcpServers"] if "mcpServers" in odata else odata
            if not isinstance(raw_servers, dict):
                raise RuntimeError(f"Expected mcpServers object in {overlay}")
            for name, cfg in raw_servers.items():
                cleaned = clean_mcp_entry(cfg)
                if cleaned:
                    servers[name] = cleaned

    return {"mcpServers": servers}


def collect_rules(repo, home):
    # Later sources override same-named files from earlier sources.
    rules_by_name = {}
    sources = [
        repo / "config/antigravity/rules",
        repo / "config/claude/rules",
        home / ".claude/rules",
        home / ".config/antigravity/rules",
    ]
    for src in sources:
        if not src.is_dir() or src.is_symlink():
            continue
        for item in sorted(src.glob("*.md")):
            if item.is_symlink() or not item.is_file():
                continue
            text = item.read_text(encoding="utf-8").strip()
            if not text:
                continue
            rules_by_name[item.name] = text
    sections = list(rules_by_name.values())
    combined = "\n\n".join(sections) + ("\n" if sections else "")
    if len(combined.encode("utf-8")) > MAX_RULE_BYTES:
        print(
            f"agy-setup: warning: combined rules exceed {MAX_RULE_BYTES} bytes",
            file=sys.stderr,
        )
    return combined


def copy_skills(repo, home, dst_skills):
    sources = [
        home / ".local/share/claude-guard/home/.claude/skills",
        home / ".agents/skills",
    ]

    copied = []
    seen = set()
    for src in sources:
        if not src.is_dir() or src.is_symlink():
            continue
        for entry in sorted(src.iterdir()):
            if entry.name.startswith(".") or entry.name in SKIP_SKILLS or entry.name in seen:
                continue
            try:
                target = entry.resolve(strict=True)
            except OSError:
                continue
            if not target.is_dir() or not (target / "SKILL.md").is_file():
                continue
            shutil.copytree(
                target,
                dst_skills / entry.name,
                symlinks=False,
                ignore_dangling_symlinks=True,
                ignore=shutil.ignore_patterns(".DS_Store", ".git", "__pycache__"),
            )
            seen.add(entry.name)
            copied.append(entry.name)

    jevgrep_skill = repo / "config/jevgrep/skill"
    if "jevgrep" not in copied and (jevgrep_skill / "SKILL.md").is_file():
        shutil.copytree(
            jevgrep_skill,
            dst_skills / "jevgrep",
            symlinks=False,
            ignore_dangling_symlinks=True,
            ignore=shutil.ignore_patterns(".DS_Store", ".git", "__pycache__"),
        )
        copied.append("jevgrep")
    return copied


def copy_agents(repo, home, dst_agents):
    copied = {}
    for src in (repo / "config/antigravity/agents", home / ".config/antigravity/agents"):
        if not src.is_dir() or src.is_symlink():
            continue
        for item in sorted(src.glob("*.md")):
            if item.is_symlink() or not item.is_file():
                continue
            shutil.copy2(item, dst_agents / item.name)
            copied[item.name] = True
    return list(copied)


def main():
    repo = pathlib.Path(os.environ["AGY_SETUP_REPO"]).resolve(strict=True)
    home = pathlib.Path.home().resolve(strict=True)
    xdg_data = pathlib.Path(os.environ.get("XDG_DATA_HOME") or (home / ".local/share"))
    jcode_data = pathlib.Path(os.environ.get("JCODE_INTEGRATIONS_DIR") or (xdg_data / "jcode-integrations"))

    gemini_dir = home / ".gemini"
    config_dir = gemini_dir / "config"
    plugins_dir = config_dir / "plugins"
    ensure_safe_dir(gemini_dir, 0o755)
    ensure_safe_dir(config_dir, 0o755)
    ensure_safe_dir(plugins_dir, 0o755)

    manifest = json.loads((repo / "config/antigravity/plugin.json").read_text(encoding="utf-8"))
    plugin_name = manifest.get("name")
    if not isinstance(plugin_name, str) or not plugin_name or "/" in plugin_name or plugin_name in (".", ".."):
        raise RuntimeError("Invalid plugin name in config/antigravity/plugin.json")

    lock_path = config_dir / ".agy-setup.lock"
    lock_fd = os.open(lock_path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        lock_info = os.fstat(lock_fd)
        if not stat.S_ISREG(lock_info.st_mode) or lock_info.st_uid != os.getuid():
            raise RuntimeError(f"Unsafe lock file: {lock_path}")
        fcntl.flock(lock_fd, fcntl.LOCK_EX)

        target_dir = plugins_dir / plugin_name
        try:
            target_info = target_dir.lstat()
        except FileNotFoundError:
            target_info = None
        if target_info is not None:
            if stat.S_ISLNK(target_info.st_mode):
                raise RuntimeError(f"Refusing symlink plugin directory: {target_dir}")
            if not stat.S_ISDIR(target_info.st_mode) or target_info.st_uid != os.getuid():
                raise RuntimeError(f"Refusing non-directory plugin path: {target_dir}")

        stage = pathlib.Path(tempfile.mkdtemp(prefix=f".{plugin_name}.stage.", dir=plugins_dir))
        backup = None
        try:
            os.chmod(stage, 0o700)
            (stage / "plugin.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

            mcp_data = build_mcp(repo, home, jcode_data)
            mcp_path = stage / "mcp_config.json"
            mcp_path.write_text(json.dumps(mcp_data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
            os.chmod(mcp_path, 0o600)

            rules_text = collect_rules(repo, home)
            if rules_text:
                (stage / "rules").mkdir(mode=0o700)
                (stage / "rules/AGENTS.md").write_text(rules_text, encoding="utf-8")

            (stage / "agents").mkdir(mode=0o700)
            agents = copy_agents(repo, home, stage / "agents")
            if not agents:
                (stage / "agents").rmdir()

            (stage / "skills").mkdir(mode=0o700)
            skills = copy_skills(repo, home, stage / "skills")
            if not skills:
                (stage / "skills").rmdir()

            # Validate staged plugin before replacing any existing installation.
            json.loads((stage / "plugin.json").read_text(encoding="utf-8"))
            json.loads(mcp_path.read_text(encoding="utf-8"))
            agy_bin = home / ".local/bin/agy"
            if agy_bin.is_file() and os.access(agy_bin, os.X_OK):
                subprocess.run([str(agy_bin), "plugin", "validate", str(stage)], check=True)

            if target_info is not None:
                backup = pathlib.Path(tempfile.mkdtemp(prefix=f".{plugin_name}.bak.", dir=plugins_dir))
                backup.rmdir()
                target_dir.rename(backup)
            try:
                stage.rename(target_dir)
            except BaseException:
                if backup is not None and backup.exists() and not target_dir.exists():
                    backup.rename(target_dir)
                raise
            if backup is not None and backup.exists():
                shutil.rmtree(backup)

            print(
                f"agy-setup: {plugin_name} установлен "
                f"(skills: {len(skills)}, agents: {len(agents)}, mcp: {len(mcp_data['mcpServers'])})"
            )
        finally:
            if stage.exists():
                shutil.rmtree(stage)
    finally:
        os.close(lock_fd)


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as exc:
        print(f"agy-setup: {exc}", file=sys.stderr)
        sys.exit(1)
PY
