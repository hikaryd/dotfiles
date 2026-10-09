#!/usr/bin/env python3
"""Install personal Pi settings, theme, footer and model overrides without touching auth."""

import json
import os
import shutil
import stat
import tempfile
import uuid
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def read_json(path: Path) -> dict:
    if not path.exists():
        return {}
    data = json.loads(path.read_text())
    if not isinstance(data, dict):
        raise ValueError(f"Expected a JSON object in {path}")
    return data


def merge(current: dict, overrides: dict) -> dict:
    result = dict(current)
    for key, value in overrides.items():
        if isinstance(value, dict) and isinstance(result.get(key), dict):
            result[key] = merge(result[key], value)
        else:
            result[key] = value
    return result


def backup_path(path: Path) -> Path:
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    return path.with_name(f"{path.name}.bak-{stamp}-{uuid.uuid4().hex[:8]}")


def write_json(path: Path, data: dict) -> None:
    if path.exists() and read_json(path) == data:
        return
    # Preserve an existing settings symlink instead of replacing it.
    target = path.resolve() if path.is_symlink() else path
    target.parent.mkdir(parents=True, exist_ok=True)
    mode = stat.S_IMODE(target.stat().st_mode) if target.exists() else 0o600
    if target.exists():
        shutil.copy2(target, backup_path(target))
    fd, temporary = tempfile.mkstemp(prefix=f".{target.name}-", dir=target.parent)
    try:
        os.fchmod(fd, mode)
        with os.fdopen(fd, "w") as stream:
            stream.write(json.dumps(data, indent=2, ensure_ascii=False) + "\n")
        os.replace(temporary, target)
    finally:
        Path(temporary).unlink(missing_ok=True)


def link_resource(target: Path, source: Path, backup_dir: Path) -> None:
    if not source.exists():
        raise FileNotFoundError(f"Missing Pi resource: {source}")
    if target.is_symlink() and target.resolve() == source.resolve():
        return
    target.parent.mkdir(parents=True, exist_ok=True)
    if target.exists() or target.is_symlink():
        # An archived extension must not remain in Pi's auto-discovery directory.
        backup_dir.mkdir(parents=True, exist_ok=True)
        target.rename(backup_dir / backup_path(target).name)
    target.symlink_to(source)


def apply(agent_dir: Path) -> None:
    agent_dir = agent_dir.expanduser().absolute()
    settings_path = agent_dir / "settings.json"
    models_path = agent_dir / "models.json"
    # Validate local configs and templates before changing anything. Never read auth.json.
    settings = merge(read_json(settings_path), read_json(ROOT / "config/pi/settings.json"))
    models = merge(read_json(models_path), read_json(ROOT / "config/pi/models.json"))
    resources = [
        (agent_dir / "themes/ayu.json", ROOT / "config/pi/themes/ayu.json"),
        (agent_dir / "extensions/dots-ayu-bar", ROOT / "config/pi/extensions/ayu-bar"),
    ]
    for _, source in resources:
        if not source.exists():
            raise FileNotFoundError(f"Missing Pi resource: {source}")
    for target, source in resources:
        link_resource(target, source, agent_dir / "backups")
    write_json(settings_path, settings)
    write_json(models_path, models)
    print(f"Pi: personal settings, Ayu theme, quota bar and model overrides installed in {agent_dir}")


if __name__ == "__main__":
    try:
        apply(Path(os.environ.get("PI_CODING_AGENT_DIR", "~/.pi/agent")))
    except (OSError, ValueError) as error:
        raise SystemExit(f"pi-apply: {error}") from error
