#!/usr/bin/env python3
"""Pick Herdr workspaces or open sesh entries as fresh Herdr shells."""

import argparse
import hashlib
import json
import shutil
import subprocess
import sys
from pathlib import Path


def command(*args):
    return subprocess.run(args, text=True, capture_output=True, check=True).stdout


def result(*args):
    payload = json.loads(command(*args))
    value = payload.get("result") if isinstance(payload, dict) else None
    if not isinstance(value, dict):
        raise TypeError(f"Некорректный ответ {' '.join(args[:2])}")
    return value


def clean_text(value):
    return (
        isinstance(value, str)
        and value
        and all(ord(char) >= 32 and ord(char) != 127 for char in value)
    )


def directory(value):
    if not clean_text(value):
        return None
    path = Path(value).expanduser().resolve()
    return str(path) if path.is_dir() else None


def session_key(source, name, path):
    identity = f"{source}\0{name}" if source == "tmux" else f"{source}\0{name}\0{path}"
    return hashlib.sha256(identity.encode()).hexdigest()[:16]


def herdr_snapshot():
    snapshot = result("herdr", "api", "snapshot").get("snapshot")
    if not isinstance(snapshot, dict) or not isinstance(
        snapshot.get("workspaces"), list
    ):
        raise TypeError("Некорректный снимок Herdr")
    return snapshot


def live_workspaces(snapshot):
    panes = snapshot.get("panes", [])
    entries = []
    for workspace in snapshot["workspaces"]:
        if not isinstance(workspace, dict):
            continue
        workspace_id = workspace.get("workspace_id")
        name = workspace.get("label")
        if not clean_text(workspace_id) or not clean_text(name):
            continue
        cwd = next(
            (
                pane.get("cwd")
                for pane in panes
                if isinstance(pane, dict)
                and pane.get("workspace_id") == workspace_id
                and directory(pane.get("cwd"))
            ),
            None,
        )
        entries.append(
            {
                "id": f"herdr:{workspace_id}",
                "source": "herdr",
                "name": name,
                "path": cwd,
                "workspace_id": workspace_id,
                "tabs": workspace.get("tab_count", 0),
                "panes": workspace.get("pane_count", 0),
                "fresh_shells": False,
                "import_key": (workspace.get("tokens") or {}).get("dots_session_id"),
            }
        )
    return entries


def sesh_entries():
    data = json.loads(command("sesh", "list", "-j"))
    if not isinstance(data, list):
        raise TypeError("Некорректный список sesh")
    entries = []
    seen = set()
    for item in data:
        if not isinstance(item, dict):
            continue
        source, name = item.get("Src"), item.get("Name")
        if source not in {"tmux", "config", "zoxide"} or not clean_text(name):
            continue
        path = directory(item.get("Path"))
        if not path:
            continue
        key = session_key(source, name, path)
        if key in seen:
            continue
        seen.add(key)
        entries.append(
            {
                "id": f"sesh:{source}:{key}",
                "source": source,
                "name": name,
                "path": path,
                "fresh_shells": True,
                "import_key": key,
            }
        )
    return entries


def inventory():
    snapshot = herdr_snapshot()
    return live_workspaces(snapshot) + sesh_entries()


def choose(entries):
    labels = []
    for item in entries:
        if item["source"] == "herdr":
            detail = f"Herdr {item['workspace_id']} · {item['tabs']} tabs"
        elif item["source"] == "tmux":
            detail = "tmux · copy layout as fresh shells"
        else:
            detail = f"{item['source']} · fresh shell"
        labels.append(f"{item['name']}  [{detail}]  {item['path'] or ''}")
    if shutil.which("gum"):
        picker = [
            "gum",
            "filter",
            "--limit",
            "1",
            "--no-sort",
            "--placeholder",
            "Session / workspace",
        ]
    elif shutil.which("fzf"):
        picker = ["fzf", "--no-sort", "--prompt", "Session > "]
    else:
        raise ValueError("Нужен gum или fzf; можно передать id напрямую")
    picked = subprocess.run(
        picker,
        input="\n".join(labels) + "\n",
        text=True,
        stdout=subprocess.PIPE,
        check=False,
    )
    if picked.returncode:
        return None
    return next(
        (
            item
            for item, label in zip(entries, labels)
            if label == picked.stdout.rstrip("\n")
        ),
        None,
    )


def tmux_layout(name):
    windows = []
    for line in command(
        "tmux",
        "list-windows",
        "-a",
        "-F",
        "#{session_name}\t#{window_id}\t#{window_index}\t#{window_name}",
    ).splitlines():
        fields = line.split("\t", 3)
        if len(fields) != 4 or fields[0] != name or not clean_text(fields[3]):
            continue
        windows.append(
            {"id": fields[1], "index": fields[2], "label": fields[3], "panes": []}
        )
    by_id = {item["id"]: item for item in windows}
    for line in command(
        "tmux",
        "list-panes",
        "-a",
        "-F",
        "#{session_name}\t#{window_id}\t#{pane_index}\t#{pane_current_path}",
    ).splitlines():
        fields = line.split("\t", 3)
        if len(fields) != 4 or fields[0] != name or fields[1] not in by_id:
            continue
        by_id[fields[1]]["panes"].append((int(fields[2]), directory(fields[3])))
    windows.sort(key=lambda item: int(item["index"]))
    for window in windows:
        window["panes"].sort()
    if not windows or any(not window["panes"] for window in windows):
        raise ValueError(f"tmux-сессия {name} изменилась; откройте список заново")
    return windows


def existing_workspace(entry, snapshot, windows):
    workspaces = live_workspaces(snapshot)
    matches = [item for item in workspaces if item["import_key"] == entry["import_key"]]
    if not matches:
        matches = [
            item
            for item in workspaces
            if item["name"] == entry["name"] and item["path"] == entry["path"]
        ]
    if not matches and windows:
        expected = [(window["label"], len(window["panes"])) for window in windows]
        for item in workspaces:
            if item["name"] != entry["name"]:
                continue
            tabs = sorted(
                (
                    tab
                    for tab in snapshot.get("tabs", [])
                    if isinstance(tab, dict)
                    and tab.get("workspace_id") == item["workspace_id"]
                ),
                key=lambda tab: tab.get("number", 0),
            )
            if [(tab.get("label"), tab.get("pane_count")) for tab in tabs] == expected:
                matches.append(item)
    if len(matches) > 1:
        raise ValueError(
            "Несколько рабочих пространств подходят сессии; выберите Herdr id"
        )
    return matches[0]["workspace_id"] if matches else None


def open_entry(entry):
    if entry["source"] == "herdr":
        result("herdr", "workspace", "focus", entry["workspace_id"])
        return
    windows = tmux_layout(entry["name"]) if entry["source"] == "tmux" else []
    snapshot = herdr_snapshot()
    workspace = existing_workspace(entry, snapshot, windows)
    if workspace:
        result("herdr", "workspace", "focus", workspace)
        return
    path = directory(entry["path"])
    if not path:
        raise ValueError("Каталог сессии больше не существует; откройте список заново")
    first_path = (windows[0]["panes"][0][1] or path) if windows else path
    created = result(
        "herdr",
        "workspace",
        "create",
        "--cwd",
        first_path,
        "--label",
        entry["name"],
        "--no-focus",
    )
    workspace = created["workspace"]["workspace_id"]
    command(
        "herdr",
        "workspace",
        "report-metadata",
        workspace,
        "--source",
        "dots-sessions",
        "--token",
        f"dots_session_id={entry['import_key']}",
    )
    if windows:
        first_tab = created["tab"]["tab_id"]
        first_pane = created["root_pane"]["pane_id"]
        for index, window in enumerate(windows):
            if index == 0:
                tab, root_pane = first_tab, first_pane
                result("herdr", "tab", "rename", tab, window["label"])
            else:
                tab_created = result(
                    "herdr",
                    "tab",
                    "create",
                    "--workspace",
                    workspace,
                    "--cwd",
                    window["panes"][0][1] or path,
                    "--label",
                    window["label"],
                    "--no-focus",
                )
                root_pane = tab_created["root_pane"]["pane_id"]
            for _, pane_path in window["panes"][1:]:
                result(
                    "herdr",
                    "pane",
                    "split",
                    root_pane,
                    "--direction",
                    "right",
                    "--cwd",
                    pane_path or path,
                    "--no-focus",
                )
    result("herdr", "workspace", "focus", workspace)
    suffix = "; процессы tmux не переносились" if windows else ""
    print(
        f"{entry['name']}: создано рабочее пространство Herdr с пустыми shell{suffix}"
    )


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="action", required=True)
    sub.add_parser("list").add_argument("--json", action="store_true")
    sub.add_parser("open").add_argument("target", nargs="?")
    args = parser.parse_args(argv)
    try:
        entries = inventory()
        if args.action == "list":
            if args.json:
                print(json.dumps(entries, ensure_ascii=False, indent=2))
            else:
                for item in entries:
                    suffix = " · fresh shells" if item["fresh_shells"] else ""
                    print(f"{item['id']}  {item['name']} [{item['source']}{suffix}]")
            return 0
        if args.target:
            matches = [
                item
                for item in entries
                if args.target in (item["id"], item["name"], item["path"])
            ]
            if len(matches) > 1:
                herdr = [item for item in matches if item["source"] == "herdr"]
                matches = herdr if len(herdr) == 1 else matches
            if len(matches) != 1:
                raise ValueError(
                    "Сессия не найдена или имя неоднозначно; используйте id из list"
                )
            selected = matches[0]
        else:
            selected = choose(entries)
        if selected:
            open_entry(selected)
        return 0
    except (
        OSError,
        ValueError,
        KeyError,
        TypeError,
        subprocess.CalledProcessError,
    ) as error:
        print(f"dots sessions: {error}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        return 130


if __name__ == "__main__":
    sys.exit(main())
