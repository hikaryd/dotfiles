#!/usr/bin/env python3
"""Локальные проекты из живых tmux panes и зарегистрированных Git worktrees."""

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def run(*args, check=False):
    return subprocess.run(args, text=True, capture_output=True, check=check)


def registry_path():
    return (
        Path(os.environ.get("XDG_CONFIG_HOME", str(Path.home() / ".config")))
        / "dots/projects.json"
    )


def allowed(path):
    p = Path(path).expanduser()
    config = registry_path().parent.parent
    if any(ord(c) < 32 or ord(c) == 127 for c in str(p)):
        return False
    if ".omx" in p.parts or p == Path.home() or p == config or config in p.parents:
        return False
    p = p.resolve()
    return p.is_dir() and ".omx" not in p.parts and p != Path.home()


def git_root(path):
    if not allowed(path):
        return None
    result = run("git", "-C", str(path), "rev-parse", "--show-toplevel")
    root = result.stdout.strip()
    return (
        str(Path(root).resolve()) if result.returncode == 0 and allowed(root) else None
    )


def project_id(path):
    return hashlib.sha256(path.encode()).hexdigest()[:12]


def session_name(path):
    slug = re.sub(r"[^a-zA-Z0-9_-]+", "-", Path(path).name).strip("-") or "project"
    return f"{slug[:45]}-{project_id(path)}"


def live_panes():
    result = run(
        "tmux",
        "list-panes",
        "-a",
        "-F",
        "#{session_id}\t#{session_name}\t#{window_id}\t#{pane_id}\t#{session_attached}\t#{pane_current_command}\t#{pane_active}\t#{pane_current_path}",
    )
    panes = []
    for line in result.stdout.splitlines():
        fields = line.split("\t", 7)
        if len(fields) == 8:
            session, name, window, pane, attached, command, active, cwd = fields
            panes.append(
                {
                    "session": session,
                    "name": name,
                    "window": window,
                    "pane": pane,
                    "attached": attached != "0",
                    "cwd": cwd,
                    "command": command,
                    "active": active == "1",
                }
            )
    return panes


def validate_registry(data):
    if (
        not isinstance(data, dict)
        or type(data.get("version")) is not int
        or data["version"] != 1
        or not isinstance(data.get("projects"), list)
    ):
        raise ValueError(
            "Некорректная схема registry projects.json (ожидается version=1)."
        )
    for item in data["projects"]:
        if (
            not isinstance(item, dict)
            or not isinstance(item.get("id"), str)
            or not item["id"]
            or not isinstance(item.get("path"), str)
            or not Path(item["path"]).is_absolute()
            or any(ord(c) < 32 or ord(c) == 127 for c in item["path"])
        ):
            raise ValueError("Некорректная запись проекта в projects.json.")
    return data


def saved_paths():
    try:
        content = registry_path().read_text()
    except FileNotFoundError:
        return []
    data = validate_registry(json.loads(content))
    return [item["path"] for item in data["projects"]]


def inventory():
    roots = {}
    cache = {}

    def root_for(path):
        if path not in cache:
            cache[path] = git_root(path)
        return cache[path]

    for pane in live_panes():
        root = root_for(pane["cwd"])
        if root:
            roots.setdefault(root, []).append(pane)
    for path in saved_paths():
        root = root_for(path)
        if root:
            roots.setdefault(root, [])
    for root in list(roots):
        result = run("git", "-C", root, "worktree", "list", "--porcelain", "-z")
        for field in result.stdout.split("\0"):
            if field.startswith("worktree "):
                path = field[9:]
                worktree = root_for(path)
                if worktree:
                    roots.setdefault(worktree, [])
    projects = []
    for path, panes in sorted(roots.items()):
        # Attached sessions first; canonical project session beats old orchestration.
        panes.sort(
            key=lambda p: (
                not p["attached"],
                p["name"] != Path(path).name,
                Path(p["command"]).name.lstrip("-")
                in {"", "zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh", "csh"},
                not p["active"],
                p["session"],
                p["pane"],
            )
        )
        projects.append(
            {
                "id": project_id(path),
                "name": Path(path).name,
                "path": path,
                "panes": panes,
            }
        )
    return projects


def save(projects):
    destination = registry_path()
    data = validate_registry(
        {
            "version": 1,
            "projects": [{"id": p["id"], "path": p["path"]} for p in projects],
        }
    )
    # Refuse to overwrite unreadable/invalid data, including direct save callers.
    saved_paths()
    destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".projects-", dir=destination.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(
                data,
                stream,
                ensure_ascii=False,
                indent=2,
            )
            stream.write("\n")
        os.replace(temporary, destination)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def pick(projects):
    lines = [
        f"{p['id']}  {'●' if p['panes'] else '○'} {p['name']}  {p['path']}"
        for p in projects
    ]
    if not lines:
        raise ValueError("Нет проектов: откройте Git-проект в tmux или передайте путь.")
    if shutil.which("gum"):
        command = ["gum", "filter", "--placeholder", "Проект / worktree"]
    elif shutil.which("fzf"):
        command = ["fzf", "--prompt", "Проект > "]
    else:
        raise ValueError("Нужен gum или fzf; можно передать путь напрямую.")
    result = subprocess.run(
        command,
        input="\n".join(lines) + "\n",
        text=True,
        stdout=subprocess.PIPE,
        check=False,
    )
    if result.returncode:
        return None
    return next(
        (
            p
            for p, line in zip(projects, lines, strict=True)
            if result.stdout.removesuffix("\n") == line
        ),
        None,
    )


def focus(pane):
    run("tmux", "select-window", "-t", pane["window"], check=True)
    run("tmux", "select-pane", "-t", pane["pane"], check=True)
    command = "switch-client" if os.environ.get("TMUX") else "attach-session"
    subprocess.run(["tmux", command, "-t", pane["session"]], check=True)


def open_project(target=None):
    projects = inventory()
    selected = (
        next((p for p in projects if target in (p["id"], p["path"])), None)
        if target
        else pick(projects)
    )
    if target and not selected:
        root = git_root(target)
        if not root:
            raise ValueError(f"Не найден Git-проект: {target}")
        selected = next(
            (p for p in projects if p["path"] == root), {"path": root, "panes": []}
        )
    if selected is None:
        return
    # Always re-read live panes, never trust pane IDs persisted on disk.
    current = next((p for p in inventory() if p["path"] == selected["path"]), selected)
    if current["panes"]:
        focus(current["panes"][0])
        return
    path = current["path"]
    name = session_name(path)
    # new-session refuses an existing name. Do not take over an unrelated session.
    created = run(
        "tmux",
        "new-session",
        "-d",
        "-P",
        "-F",
        "#{session_id}\t#{window_id}\t#{pane_id}",
        "-s",
        name,
        "-n",
        "editor",
        "-c",
        path,
        *(["nvim"] if shutil.which("nvim") else []),
        check=True,
    )
    session, window, pane = created.stdout.strip().split("\t")
    for label in ("shell", "logs"):
        run(
            "tmux",
            "new-window",
            "-d",
            "-t",
            session + ":",
            "-n",
            label,
            "-c",
            path,
            check=True,
        )
    focus({"session": session, "window": window, "pane": pane})


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("list").add_argument("--json", action="store_true")
    commands.add_parser("refresh")
    commands.add_parser("open").add_argument("target", nargs="?")
    args = parser.parse_args(argv)
    try:
        if args.command == "open":
            open_project(args.target)
        else:
            projects = inventory()
            if args.command == "refresh":
                save(projects)
                print(f"{len(projects)} проектов: {registry_path()}")
            elif args.json:
                print(json.dumps(projects, ensure_ascii=False, indent=2))
            else:
                for p in projects:
                    print(f"{p['id']}  {'●' if p['panes'] else '○'} {p['path']}")
        return 0
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"dots projects: {error}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        return 130


if __name__ == "__main__":
    sys.exit(main())
