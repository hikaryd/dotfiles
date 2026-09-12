#!/usr/bin/env python3
"""Снимок открытых проектов: Git, tmux-процессы и локальные TCP listeners."""

import argparse
import json
import os
import shutil
import subprocess
import sys
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from pathlib import Path

PROJECTS = Path(__file__).resolve().with_name("dots-projects.py")


def command(argv, timeout=5):
    return subprocess.run(
        argv,
        capture_output=True,
        timeout=timeout,
        check=False,
        env={**os.environ, "GIT_OPTIONAL_LOCKS": "0"},
    )


def text(value):
    # Не позволяем именам branch/process/file менять терминал через control chars.
    return "".join(c if c.isprintable() else "?" for c in str(value))


def parse_git(data):
    result = {
        "state": "clean",
        "branch": "?",
        "staged": 0,
        "unstaged": 0,
        "untracked": 0,
        "conflicts": 0,
    }
    fields = iter(data.split(b"\0"))
    header_seen = False
    for entry in fields:
        if not entry:
            continue
        if entry.startswith(b"## "):
            header_seen = True
            branch = entry[3:].decode("utf-8", "replace").split("...", 1)[0]
            for prefix in ("No commits yet on ", "Initial commit on "):
                branch = branch.removeprefix(prefix)
            result["branch"] = branch
            continue
        if len(entry) < 3 or entry[2:3] != b" ":
            raise ValueError("Unexpected git status record")
        xy = entry[:2]
        if xy == b"??":
            result["untracked"] += 1
        elif xy in {b"DD", b"AU", b"UD", b"UA", b"DU", b"AA", b"UU"}:
            result["conflicts"] += 1
        else:
            result["staged"] += xy[0:1] not in {b" ", b"?", b"!"}
            result["unstaged"] += xy[1:2] not in {b" ", b"?", b"!"}
        # Porcelain -z rename/copy emits destination then a separate source record.
        if (b"R" in xy or b"C" in xy) and next(fields, None) is None:
            raise ValueError("Missing rename source")
    if not header_seen:
        raise ValueError("Missing git branch header")
    if any(result[key] for key in ("staged", "unstaged", "untracked", "conflicts")):
        result["state"] = "dirty"
    return result


def git_status(path):
    try:
        # status may invoke arbitrary clean/process filters while hashing a
        # modified file. Never execute repository filters in a read-only dashboard.
        filters = command(
            [
                "git",
                "-C",
                path,
                "config",
                "--null",
                "--get-regexp",
                r"^filter\..*\.(clean|process)$",
            ]
        )
        if filters.returncode not in (0, 1):
            return {
                "state": "unknown",
                "branch": "?",
                "error": "git config unavailable",
            }
        if any(record.partition(b"\n")[2] for record in filters.stdout.split(b"\0")):
            branch = command(
                ["git", "-C", path, "symbolic-ref", "--short", "-q", "HEAD"]
            )
            return {
                "state": "unknown",
                "branch": branch.stdout.decode("utf-8", "replace").strip()
                or "detached",
                "error": "executable Git filters configured; status skipped",
            }
        result = command(
            [
                "git",
                "--no-optional-locks",
                "-C",
                path,
                "-c",
                "core.fsmonitor=false",
                "status",
                "--porcelain=v1",
                "-z",
                "--branch",
                "--untracked-files=normal",
                "--ignore-submodules=dirty",
            ],
            timeout=5,
        )
        if result.returncode:
            return {"state": "unknown", "branch": "?", "error": "git status failed"}
        return parse_git(result.stdout)
    except (OSError, ValueError, subprocess.TimeoutExpired):
        return {"state": "unknown", "branch": "?", "error": "git unavailable/timeout"}


def parse_processes(data):
    processes = {}
    for line in data.decode("utf-8", "replace").splitlines():
        parts = line.split(None, 2)
        if len(parts) == 3 and parts[0].isdigit() and parts[1].isdigit():
            processes[int(parts[0])] = {
                "parent": int(parts[1]),
                "command": Path(parts[2]).name,
            }
    return processes


def parse_lsof(data):
    """lsof -F0 records; не режем путь с пробелами или переводом строки."""
    records = {}
    pid = None
    for field in data.split(b"\0"):
        field = field.lstrip(b"\n")
        if not field:
            continue
        kind, value = field[:1], field[1:].decode("utf-8", "replace")
        if kind == b"p":
            pid = int(value) if value.isdigit() else None
            if pid is not None:
                records.setdefault(pid, {"command": "?", "names": []})
        elif pid is not None and kind == b"c":
            records[pid]["command"] = value
        elif pid is not None and kind == b"n":
            records[pid]["names"].append(value)
    return records


def root_for_cwd(cwd, projects):
    candidate = Path(cwd).resolve()
    matches = [
        p
        for p in projects
        if candidate == Path(p["path"]) or Path(p["path"]) in candidate.parents
    ]
    return (
        max(matches, key=lambda p: len(Path(p["path"]).parts))["id"]
        if matches
        else None
    )


def ancestor_project(pid, processes, pane_projects):
    visited = set()
    while pid and pid not in visited:
        if pid in pane_projects:
            return pane_projects[pid]
        visited.add(pid)
        pid = processes.get(pid, {}).get("parent", 0)
    return None


def local_ports(projects):
    warnings = []
    try:
        process_result = command(["ps", "-axo", "pid=,ppid=,comm="])
        processes = parse_processes(process_result.stdout)
        if process_result.returncode:
            warnings.append("Process ancestry unavailable")
    except (OSError, subprocess.TimeoutExpired):
        processes = {}
        warnings.append("Process ancestry unavailable")
    try:
        result = command(["lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn0"])
    except (OSError, subprocess.TimeoutExpired):
        return {}, [], warnings + ["TCP listeners unavailable (lsof missing/timeout)"]
    if result.returncode not in (0, 1) or (result.returncode == 1 and result.stderr):
        return {}, [], warnings + ["TCP listeners could not be inspected"]
    listeners = parse_lsof(result.stdout)
    cwd = {}
    if listeners:
        try:
            result = command(
                [
                    "lsof",
                    "-a",
                    "-p",
                    ",".join(str(pid) for pid in listeners),
                    "-d",
                    "cwd",
                    "-Fpn0",
                ]
            )
            if result.returncode not in (0, 1):
                warnings.append("Process cwd unavailable; ancestry attribution only")
            cwd = parse_lsof(result.stdout)
        except (OSError, subprocess.TimeoutExpired):
            warnings.append("Process cwd unavailable; ancestry attribution only")
    pane_ids = {pane["pane"]: p["id"] for p in projects for pane in p["panes"]}
    pane_projects = {}
    try:
        result = command(["tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{pane_pid}"])
        for line in result.stdout.decode().splitlines():
            parts = line.split("\t")
            if len(parts) == 2 and parts[1].isdigit() and parts[0] in pane_ids:
                pane_projects[int(parts[1])] = pane_ids[parts[0]]
    except (OSError, UnicodeError, subprocess.TimeoutExpired):
        warnings.append("Pane process attribution unavailable")
    attributed, unassigned = {}, []
    for pid, info in listeners.items():
        paths = cwd.get(pid, {}).get("names", [])
        project = root_for_cwd(paths[0], projects) if paths else None
        via = "cwd" if project else "pane ancestry"
        if not project and not paths:
            project = ancestor_project(pid, processes, pane_projects)
        for address in sorted(set(info["names"])):
            row = {
                "pid": pid,
                "process": info["command"],
                "address": address,
                "via": via if project else "unassigned",
            }
            if project:
                attributed.setdefault(project, []).append(row)
            else:
                unassigned.append(row)
    return attributed, unassigned, warnings


def collect(include_all=False):
    result = command([sys.executable, str(PROJECTS), "list", "--json"], timeout=20)
    if result.returncode:
        raise ValueError("Project inventory unavailable; check dots projects list")
    projects = json.loads(result.stdout)
    ports, unassigned, warnings = local_ports(projects)
    selected = [p for p in projects if include_all or p["panes"] or ports.get(p["id"])]
    with ThreadPoolExecutor(max_workers=4) as pool:
        git_results = list(pool.map(git_status, [p["path"] for p in selected]))
    rows = [
        {**p, "git": git, "ports": ports.get(p["id"], [])}
        for p, git in zip(selected, git_results, strict=True)
    ]
    rows.sort(key=lambda p: (not bool(p["panes"]), p["name"], p["path"]))
    return {
        "at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "projects": rows,
        "known_projects": len(projects),
        "unassigned_ports": unassigned,
        "warnings": warnings,
    }


def git_summary(git):
    if git["state"] == "unknown":
        return "? unknown"
    if git["state"] == "clean":
        return "clean"
    return (
        f"S{git['staged']} M{git['unstaged']} ?{git['untracked']} !{git['conflicts']}"
    )


def render(report):
    rows = report["projects"]
    print(f"PROJECTS · {report['at']} · {len(rows)}/{report['known_projects']} shown")
    print("Opened panes / local listeners · one-shot snapshot, no network")
    print(f"{'PROJECT':32} {'BRANCH':25} {'GIT':18} {'PROCESSES':23} LISTEN")
    print("─" * min(shutil.get_terminal_size((120, 30)).columns, 140))
    for row in rows:
        counts = Counter(p["command"] for p in row["panes"])
        processes = (
            ",".join(f"{text(cmd)}×{count}" for cmd, count in sorted(counts.items()))
            or "—"
        )
        ports = ",".join(sorted({p["address"] for p in row["ports"]})) or "—"
        label = f"{row['name']} [{row['id'][:4]}]"
        print(
            f"{text(label)[:32]:32} {text(row['git']['branch'])[:25]:25} {git_summary(row['git']):18} {processes[:23]:23} {text(ports)}"
        )
    if not rows:
        print("Нет открытых проектов; --all покажет сохранённые worktree.")
    print("S staged · M unstaged · ? untracked entries · ! conflicts; unknown ≠ clean")
    print(
        f"Unassigned TCP listeners: {len(report['unassigned_ports'])}; LISTEN не доказывает готовность сервиса."
    )
    for warning in report["warnings"]:
        print(f"WARN: {warning}")
    for row in rows:
        if row["git"]["state"] == "unknown":
            print(
                f"WARN: {text(row['name'])}: {text(row['git'].get('error', 'unknown'))}"
            )


def open_project(report):
    lines = [
        f"{p['id']}  {text(p['name'])}  {text(p['git']['branch'])}  {git_summary(p['git'])}"
        for p in report["projects"]
    ]
    if not lines:
        return
    if shutil.which("gum"):
        argv = [
            "gum",
            "filter",
            "--placeholder",
            "Открыть проект",
            "--no-sort",
            "--limit",
            "1",
        ]
    elif shutil.which("fzf"):
        argv = ["fzf", "--prompt", "Проект > ", "--no-multi"]
    else:
        raise ValueError("Picker requires gum or fzf")
    result = subprocess.run(
        argv,
        input="\n".join(lines) + "\n",
        text=True,
        stdout=subprocess.PIPE,
        check=False,
    )
    selected = result.stdout.removesuffix("\n")
    if result.returncode or selected not in lines:
        return
    project_id = report["projects"][lines.index(selected)]["id"]
    subprocess.run([sys.executable, str(PROJECTS), "open", project_id], check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--all", action="store_true", help="включить известные неоткрытые worktree"
    )
    parser.add_argument("--json", action="store_true")
    parser.add_argument(
        "--interactive",
        action="store_true",
        help="r refresh, o open, q quit; без polling",
    )
    args = parser.parse_args()
    if args.json and args.interactive:
        parser.error("--json нельзя совместить с --interactive")
    if args.interactive and not sys.stdin.isatty():
        parser.error("--interactive requires a terminal")
    try:
        while True:
            report = collect(args.all)
            if args.json:
                print(json.dumps(report, ensure_ascii=False, indent=2))
                return 0
            if args.interactive and sys.stdout.isatty():
                print("\033[2J\033[H", end="")
            render(report)
            if not args.interactive:
                return 0
            choice = (
                input("[Enter/r] обновить · [o] открыть проект · [q] выход: ")
                .strip()
                .lower()
            )
            if choice == "o":
                open_project(report)
                return 0
            if choice not in {"", "r"}:
                return 0
    except (OSError, ValueError, subprocess.SubprocessError):
        print(
            "dots status: сбор снимка не удался; проверьте dots projects list и локальные tools.",
            file=sys.stderr,
        )
        return 1
    except (KeyboardInterrupt, EOFError):
        return 0


if __name__ == "__main__":
    sys.exit(main())
