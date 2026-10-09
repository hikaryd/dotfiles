#!/usr/bin/env python3
"""Show Codex task names and Pi session names, with native OSC titles as a fallback.

Herdr 0.9.1 does not allow pane.updated in plugin manifests. Refresh on agent
state/focus events; an idle OSC-only rename is picked up on the next focus.
"""

import argparse
import fcntl
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
import time
import unicodedata
from contextlib import contextmanager
from pathlib import Path


SOURCE = "dots:task-titles"
AGENTS = {"codex", "pi"}
AUTOMATIC_LABELS = {"zsh", "nu", "bash", "fish", *AGENTS}
ANSI = re.compile(r"\x1b(?:\][^\x07\x1b]*(?:\x07|\x1b\\)|\[[0-?]*[ -/]*[@-~])")


def shorten(text, limit):
    if len(text) <= limit:
        return text
    prefix = text[: limit - 1]
    if " " in prefix and prefix.rfind(" ") >= limit // 2:
        prefix = prefix.rsplit(" ", 1)[0]
    return prefix.rstrip() + "…"


def clean_text(value):
    if not isinstance(value, str):
        return ""
    text = ANSI.sub("", value)
    text = "".join(c for c in text if not unicodedata.category(c).startswith("C"))
    return " ".join(text.split())


def task_title(value, limit=32, agent="codex"):
    text = clean_text(value)
    if agent == "codex":
        # Codex appends the working directory after a spaced vertical bar.
        text = text.rsplit(" | ", 1)[0].strip()
    return shorten(text, limit)


def native_title(pane):
    agent = pane.get("agent")
    text = clean_text(pane.get("terminal_title_stripped"))
    if agent == "pi":
        # Pi emits 'π - <name> - <cwd>' or 'π - <cwd>' for an unnamed session.
        if not text.startswith("π - "):
            return ""
        text = text.removeprefix("π - ")
        cwd = pane.get("cwd") or pane.get("foreground_cwd")
        if cwd:
            suffix = " - " + Path(cwd).name
            if not text.endswith(suffix):
                return ""
            text = text[:-len(suffix)]
        elif " - " in text:
            text = text.rsplit(" - ", 1)[0]
        else:
            return ""
    return task_title(text, agent=agent)


def run_herdr(*args):
    done = subprocess.run(
        [os.environ.get("HERDR_BIN_PATH", "herdr"), *args],
        capture_output=True, text=True, timeout=4, check=False,
    )
    if done.returncode:
        # Command arguments can contain task text; keep them out of diagnostics.
        raise RuntimeError(f"Herdr {args[0]} {args[1]} failed (exit {done.returncode})")
    # This verb uses Herdr's send_ok_request: exit status is its acknowledgement,
    # unlike snapshot/get/rename it intentionally prints no JSON on success.
    if args[:2] == ("pane", "report-metadata") and not done.stdout.strip():
        return {}
    response = json.loads(done.stdout)
    if "error" in response:
        raise RuntimeError(f"Herdr API error: {response['error'].get('code', 'unknown')}")
    return response["result"]


def state_file():
    socket_path = os.environ.get("HERDR_SOCKET_PATH")
    if not socket_path:
        raise ValueError("HERDR_SOCKET_PATH is required to select the session")
    root = Path(os.environ.get(
        "HERDR_PLUGIN_STATE_DIR",
        str(Path.home() / ".local/state/herdr/plugins/dots.task-titles"),
    ))
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    namespace = hashlib.sha256(socket_path.encode()).hexdigest()[:16]
    return root / f"{namespace}.json"


@contextmanager
def locked(path):
    with path.with_suffix(".lock").open("a+") as stream:
        deadline = time.monotonic() + 4
        while True:
            try:
                fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                if time.monotonic() >= deadline:
                    raise TimeoutError("task title synchronization is busy")
                time.sleep(0.05)
        yield


def read_state(path):
    try:
        state = json.loads(path.read_text())
    except FileNotFoundError:
        return {"panes": {}, "tabs": {}, "semantic": {}}
    if not isinstance(state, dict) or not all(isinstance(state.get(k), dict) for k in ("panes", "tabs")):
        raise ValueError("invalid task title state")
    if not isinstance(state.setdefault("semantic", {}), dict):
        raise ValueError("invalid semantic title state")
    return state


def write_state(path, state):
    fd, temporary = tempfile.mkstemp(prefix=".titles-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(state, stream, ensure_ascii=False)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        Path(temporary).unlink(missing_ok=True)


def session_identity(pane):
    session = pane.get("agent_session") or {}
    return session.get("value") if session.get("agent") == pane.get("agent") else None


def set_title(snapshot, state, pane_id, title, session_id=None, agent="codex"):
    """Store a task name from the working agent, bound to its terminal/session."""
    pane = next((p for p in snapshot["panes"] if p["pane_id"] == pane_id), None)
    if agent not in AGENTS or pane is None or pane.get("agent") != agent:
        raise ValueError(f"the explicit pane must contain a live {agent} agent")
    terminal_id = pane.get("terminal_id")
    if not terminal_id:
        raise ValueError("pane has no terminal identity")
    observed_session = session_identity(pane)
    if session_id and observed_session and session_id != observed_session:
        raise ValueError(f"{agent} session does not match the target pane")
    empty_pi_name = agent == "pi" and title == ""
    title = task_title(title, agent=agent)
    if not title and not empty_pi_name:
        raise ValueError("task title must not be empty")
    state.setdefault("semantic", {})[pane_id] = {
        "title": title, "terminal_id": terminal_id,
        "session_id": session_id or observed_session, "agent": agent,
    }


def semantic_titles(panes, state):
    """Discard names owned by an exited/replaced terminal or conversation."""
    semantic = state.setdefault("semantic", {})
    for pane_id, record in list(semantic.items()):
        pane = panes.get(pane_id)
        if (pane is None or pane.get("agent") != record.get("agent", "codex")
                or pane.get("terminal_id") != record["terminal_id"]):
            del semantic[pane_id]
            continue
        observed_session = session_identity(pane)
        if observed_session and record.get("session_id") not in (None, observed_session):
            del semantic[pane_id]
        elif observed_session and not record.get("session_id"):
            record["session_id"] = observed_session
    return {
        # An explicit empty Pi name masks the previous OSC title during a switch/quit.
        pane_id: task_title(semantic[pane_id]["title"], agent=pane["agent"]) if pane_id in semantic else native_title(pane)
        for pane_id, pane in panes.items() if pane.get("agent") in AGENTS
    }


def synchronize(snapshot, state, command, save=lambda: None):
    panes = {p["pane_id"]: p for p in snapshot["panes"]}
    tabs = {t["tab_id"]: t for t in snapshot["tabs"]}
    focused = {layout["tab_id"]: layout.get("focused_pane_id") for layout in snapshot["layouts"]}
    state["panes"] = {key: value for key, value in state["panes"].items() if key in panes}
    state["tabs"] = {key: value for key, value in state["tabs"].items() if key in tabs}
    desired_titles = semantic_titles(panes, state)

    for pane_id, pane in panes.items():
        previous = state["panes"].get(pane_id)
        title = desired_titles.get(pane_id, "")
        # Native/manual names and another integration's presentation take priority.
        if pane.get("label") or (pane.get("title") and pane["title"] != previous):
            title = ""
        if not title:
            if previous is not None:
                command("pane", "report-metadata", pane_id, "--source", SOURCE, "--seq", str(time.time_ns()), "--clear-title")
                state["panes"].pop(pane_id)
                save()
            continue
        if pane.get("title") != title:
            command("pane", "report-metadata", pane_id, "--source", SOURCE, "--seq", str(time.time_ns()), "--agent", pane["agent"], "--title", title)
            state["panes"][pane_id] = title
            save()

    for tab_id, tab in tabs.items():
        record = state["tabs"].get(tab_id)
        label = tab["label"]
        if record and label != record.get("last"):
            # Once the user renames this tab, never reacquire it automatically.
            record["manual"] = True
        if record and record.get("manual"):
            continue
        if record is None and label not in AUTOMATIC_LABELS and not label.isdecimal():
            continue
        candidates = [p for p in panes.values() if p["tab_id"] == tab_id and p.get("agent") in AGENTS]
        named = [p for p in candidates if desired_titles.get(p["pane_id"])]
        if not named:
            if record and label != record["original"]:
                command("tab", "rename", tab_id, record["original"])
                record["last"] = record["original"]
                save()
            continue
        chosen = next((p for p in named if p["pane_id"] == focused.get(tab_id)), None)
        if chosen is None and record:
            chosen = next((p for p in named if p["pane_id"] == record.get("pane")), None)
        chosen = chosen or named[0]
        suffix = f" · {len(candidates)}" if len(candidates) > 1 else ""
        desired = shorten(desired_titles[chosen["pane_id"]], 38 - len(suffix)) + suffix
        if record is None:
            record = {"original": label, "last": label}
            state["tabs"][tab_id] = record
        record["pane"] = chosen["pane_id"]
        if desired != label:
            command("tab", "rename", tab_id, desired)
            record["last"] = desired
            save()
    save()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="action", required=True)
    actions.add_parser("sync")
    setter = actions.add_parser("set")
    setter.add_argument("--pane", required=True)
    setter.add_argument("--title", required=True)
    setter.add_argument("--session-id")
    setter.add_argument("--agent", choices=sorted(AGENTS), default="codex")
    args = parser.parse_args()
    path = state_file()
    with locked(path):
        state = read_state(path)
        snapshot = run_herdr("api", "snapshot")["snapshot"]
        if args.action == "set":
            set_title(snapshot, state, args.pane, args.title, args.session_id, args.agent)
            write_state(path, state)
        synchronize(snapshot, state, run_herdr, lambda: write_state(path, state))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"Herdr task titles: {error}", file=sys.stderr)
        sys.exit(1)
