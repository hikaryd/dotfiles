#!/usr/bin/env python3
"""Give the working Codex agent a pane-bound command for a semantic task title."""

import json
import os
from pathlib import Path
import shlex
import sys


TITLE_SCRIPT = Path(__file__).resolve().parents[1] / "config/herdr/plugins/task-titles/titles.py"


def hook_context(payload, environment):
    pane = environment.get("HERDR_PANE_ID")
    socket = environment.get("HERDR_SOCKET_PATH")
    session = payload.get("session_id")
    if not pane or not socket or not isinstance(session, str) or not session:
        return None  # This optional presentation integration applies only in Herdr.
    if payload.get("hook_event_name") != "UserPromptSubmit":
        return None
    transcript = payload.get("transcript_path")
    if transcript:
        try:
            with open(transcript) as stream:
                first = json.loads(stream.readline(131072))
        except FileNotFoundError:
            first = {}  # New sessions can receive a prompt before the transcript exists.
        source = first.get("payload", {}).get("source")
        if isinstance(source, dict) and "subagent" in source:
            return None
    command = shlex.join([
        "env", f"HERDR_SOCKET_PATH={socket}", "python3", str(TITLE_SCRIPT),
        "set", "--pane", pane, "--session-id", session, "--title",
    ])
    return (
        "Herdr task title: after understanding the task, set a concise Russian title "
        "by running the command below with one shell-quoted title argument. "
        "Use the concrete subject/problem first, append a known ticket ID at the end if it fits; "
        "2–6 words, at most 32 characters. Examples: 'Переоткрытие заявки · 3812844', "
        "'НСИ · Ложное увольнение', 'СБ · Создание проверки'. "
        "Keep the established subject for follow-ups such as creating an MR, testing, "
        "or continuing work; update the title when you learn the actual problem or "
        "the user changes the subject. Generic task verbs alone do not identify a task. "
        "Use only information already in this conversation; omit personal data and secrets. "
        "This changes presentation only; a title failure must not block the user's work. "
        "Only the main agent sets this title; do not delegate it to subagents.\n"
        + command + " 'Короткое название задачи'"
    )


def main():
    if sys.argv[1:] != ["hook"]:
        raise ValueError("usage: codex-task-title.py hook")
    context = hook_context(json.load(sys.stdin), os.environ)
    if context:
        print(json.dumps({"hookSpecificOutput": {
            "hookEventName": "UserPromptSubmit", "additionalContext": context,
        }}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, TypeError):
        # Optional UI integration: keep the failure visible without blocking a prompt.
        print(json.dumps({"systemMessage": "Herdr task title hook failed; title context was not added."}))
