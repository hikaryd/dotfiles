#!/usr/bin/env python3
"""Install the task-title prompt hook without changing other Codex hooks."""

import json
import os
from pathlib import Path
import runpy
import shlex
import shutil
import tempfile
import time
import tomllib

ROOT = Path(__file__).resolve().parents[1]
MARKER = "dots-task-title"
COMMAND = f"python3 {shlex.quote(str(ROOT / 'scripts/codex-task-title.py'))} hook # {MARKER}"


def write(path, content):
    """Atomically replace a changed file, preserving a private backup and mode."""
    if path.is_symlink():
        raise ValueError(f"Expected a regular file: {path}")
    previous = path.read_bytes() if path.exists() else None
    encoded = content.encode()
    if encoded == previous:
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    if previous is not None:
        backup = Path.home() / ".config/dots-backups/task-titles" / str(time.time_ns())
        backup.mkdir(parents=True, mode=0o700)
        shutil.copy2(path, backup / path.name)
        (backup / path.name).chmod(0o600)
    mode = path.stat().st_mode & 0o777 if path.exists() else 0o600
    descriptor, temporary = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.")
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(encoded)
        os.chmod(temporary, mode)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def is_managed(command):
    return isinstance(command, str) and command.rstrip().endswith(f"# {MARKER}")


def merge_hook(original):
    data = json.loads(original) if original else {}
    groups = data.setdefault("hooks", {}).setdefault("UserPromptSubmit", [])
    expected = {"type": "command", "command": COMMAND, "timeout": 5}
    matches = []
    for group in groups:
        for index, entry in enumerate(group["hooks"]):
            if is_managed(entry.get("command")):
                matches.append((group, index))
    if len(matches) > 1:
        raise ValueError("Multiple managed task-title hooks found; configuration unchanged")
    if matches:
        group, index = matches[0]
        # Leave unrelated entries, group metadata, and hook positions intact.
        group["hooks"][index] = expected
    else:
        groups.append({"hooks": [expected]})
    return json.dumps(data, indent=2, ensure_ascii=False) + "\n"


def other_states(entries, source):
    return {
        entry["key"]: (entry["currentHash"], entry["trustStatus"], entry["enabled"])
        for entry in entries
        if not (Path(entry["sourcePath"]).resolve() == source and is_managed(entry.get("command")))
    }


def main():
    binary = shutil.which("codex")
    if binary is None:
        raise ValueError("Codex CLI is required to validate hook trust before installation")
    helpers = runpy.run_path(str(ROOT / "scripts/abide-apply.py"))
    list_hooks, replace_table = helpers["list_codex_hooks"], helpers["replace_table"]
    directory = Path(os.environ.get("CODEX_CONFIG_DIR") or os.environ.get("CODEX_HOME") or Path.home() / ".codex").expanduser().resolve()
    hooks_path, config_path = directory / "hooks.json", directory / "config.toml"
    for path in (hooks_path, config_path):
        if path.is_symlink():
            raise ValueError(f"Expected a regular file: {path}")
    original_hooks = hooks_path.read_text() if hooks_path.exists() else ""
    original_config = config_path.read_text() if config_path.exists() else ""
    updated_hooks = merge_hook(original_hooks)
    parsed = tomllib.loads(original_config)
    updated_config = original_config
    if parsed.get("features", {}).get("hooks") is not True:
        if "features" in parsed and "[features]" not in original_config:
            raise ValueError("Nonstandard features table; configuration unchanged")
        updated_config = replace_table(updated_config, "features", {"hooks": True})
    tomllib.loads(updated_config)
    source = hooks_path.resolve()
    for path, original in ((hooks_path, original_hooks), (config_path, original_config)):
        if (path.read_text() if path.exists() else "") != original:
            raise ValueError(f"Configuration changed during installation: {path}")
    write(config_path, updated_config)
    # hooks/list returns no entries while the feature is disabled. Take the
    # baseline after enabling discovery, before adding or trusting our hook.
    baseline = list_hooks(binary, directory)
    if (hooks_path.read_text() if hooks_path.exists() else "") != original_hooks:
        raise ValueError("Codex hooks changed during discovery")
    write(hooks_path, updated_hooks)
    entries = list_hooks(binary, directory)
    ours = [entry for entry in entries if Path(entry["sourcePath"]).resolve() == source and entry.get("command") == COMMAND]
    if len(ours) != 1:
        raise ValueError("Codex did not discover the single expected task-title hook")
    if other_states(baseline, source) != other_states(entries, source):
        raise ValueError("Other hook states changed; task-title trust was not granted")
    entry = ours[0]
    current = config_path.read_text()
    parsed = tomllib.loads(current)
    state = parsed.get("hooks", {}).get("state", {})
    existing = state.get(entry["key"], {})
    table = "hooks.state." + json.dumps(entry["key"])
    updated = current
    if existing.get("trusted_hash") != entry["currentHash"] or existing.get("enabled") is not True:
        if existing and f"[{table}]" not in current:
            raise ValueError("Nonstandard task-title trust table; trust configuration unchanged")
        updated = replace_table(current, table, {"trusted_hash": entry["currentHash"], "enabled": True})
    updated_parsed = tomllib.loads(updated)
    previous_other = {key: value for key, value in state.items() if key != entry["key"]}
    updated_other = {key: value for key, value in updated_parsed.get("hooks", {}).get("state", {}).items() if key != entry["key"]}
    if previous_other != updated_other:
        raise ValueError("Refusing to change another hook's trust configuration")
    if config_path.read_text() != current:
        raise ValueError("Codex configuration changed during trust setup")
    write(config_path, updated)
    verified = list_hooks(binary, directory)
    approved = [item for item in verified if item["key"] == entry["key"] and item.get("command") == COMMAND and Path(item["sourcePath"]).resolve() == source]
    if len(approved) != 1 or approved[0]["trustStatus"] != "trusted" or not approved[0]["enabled"]:
        raise ValueError("Codex did not confirm enabled/trusted task-title hook")
    if other_states(baseline, source) != other_states(verified, source):
        raise ValueError("Other hook states changed; inspect the configuration backup")
    print(f"Task titles: enabled/trusted UserPromptSubmit hook installed in {hooks_path}; other hook states preserved")


if __name__ == "__main__":
    main()
