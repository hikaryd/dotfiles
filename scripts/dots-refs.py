#!/usr/bin/env python3
"""Выбор ссылок из панели tmux; захват остаётся только в памяти."""

import argparse
import json
import os
import re
import selectors
import shutil
import subprocess
import sys
import time
import unicodedata
from pathlib import Path
from urllib.parse import unquote, urlsplit

MAX_BYTES = 1024 * 1024


def sanitize(text):
    # OSC/DCS/SOS/PM/APC, including truncated sequences, never reach the picker.
    text = re.sub(
        r"(?:\x1b\]|\x9d).*?(?:\x07|\x1b\\|\x9c|$)", "", text, flags=re.DOTALL
    )
    text = re.sub(
        r"(?:\x1b[P^_X]|[\x90\x98\x9e\x9f]).*?(?:\x1b\\|\x9c|$)",
        "",
        text,
        flags=re.DOTALL,
    )
    text = re.sub(r"(?:\x1b\[|\x9b)[0-?]*[ -/]*[@-~]", "", text)
    text = re.sub(r"\x1b[ -/]*[@-~]", "", text)
    return "".join(
        c for c in text if c in "\n\t" or not unicodedata.category(c).startswith("C")
    )


def safe_url(value):
    decoded = unquote(value)
    if any(c.isspace() or unicodedata.category(c).startswith("C") for c in decoded):
        return False
    try:
        parsed = urlsplit(value)
        return bool(
            parsed.scheme in {"http", "https"}
            and parsed.hostname
            and not parsed.username
            and not parsed.password
            and "@" not in parsed.netloc
            and "\\" not in value
        )
    except ValueError:
        return False


def extract(text, cwd):
    if len(text.encode("utf-8")) > MAX_BYTES:
        raise ValueError("Слишком большой захват (максимум 1 MiB).")
    text = sanitize(text)
    found = []
    seen = set()

    def add(item):
        key = (item["kind"], item["value"])
        if key not in seen:
            seen.add(key)
            found.append(item)

    for match in re.finditer(r'https?://[^\s<>"\x27]+', text):
        value = match.group().rstrip(".,;:!)]}")
        if safe_url(value):
            add({"kind": "url", "value": value})
    for match in re.finditer(
        r'\b(?:request|correlation|trace)[-_ ]?id["\x27]?\s*[:=]\s*["\x27]?([A-Za-z0-9][A-Za-z0-9_.:-]{0,255})',
        text,
        re.IGNORECASE,
    ):
        add({"kind": "id", "value": match[1]})
    for match in re.finditer(
        r"\b[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\b", text, re.IGNORECASE
    ):
        add({"kind": "id", "value": match.group()})
    paths = list(re.finditer(r'File "([^"\n]{1,4096})", line ([0-9]{1,9})', text))
    paths += list(
        re.finditer(
            r'(?<![^\s\"\x27])([^\s:"\x27<>]{1,4096}):([0-9]{1,9})(?::[0-9]+)?\b', text
        )
    )
    for match in paths:
        raw, line = match[1], int(match[2])
        if not line or "://" in raw:
            continue
        path = Path(raw).expanduser()
        if not path.is_absolute():
            path = cwd / path
        try:
            path = path.resolve()
            if path.is_file() and not any(
                unicodedata.category(c).startswith("C") for c in str(path)
            ):
                add(
                    {
                        "kind": "file",
                        "value": f"{path}:{line}",
                        "path": str(path),
                        "line": line,
                    }
                )
        except (OSError, ValueError):
            continue
    return found


def read_command(command, *, limit, timeout=5):
    process = subprocess.Popen(
        command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL
    )
    assert process.stdout is not None
    deadline = time.monotonic() + timeout
    try:
        data = bytearray()
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not selector.select(remaining):
                    raise subprocess.TimeoutExpired(command, timeout)
                part = os.read(
                    process.stdout.fileno(), min(65536, limit + 1 - len(data))
                )
                data.extend(part)
                if len(data) > limit:
                    raise ValueError("Превышен лимит вывода команды.")
                if not part:
                    break
        code = process.wait(timeout=max(0.01, deadline - time.monotonic()))
        if code:
            raise subprocess.CalledProcessError(code, command)
        return data.decode("utf-8", errors="replace")
    finally:
        if process.poll() is None:
            process.kill()
        process.wait()
        process.stdout.close()


def capture(pane, lines):
    if not re.fullmatch(r"%[0-9]+", pane or "") or not 1 <= lines <= 5000:
        raise ValueError("Нужна исходная tmux pane %id и lines в диапазоне 1..5000.")
    cwd = read_command(
        ["tmux", "display-message", "-p", "-t", pane, "#{pane_current_path}"],
        limit=16384,
    ).rstrip("\n")
    if not cwd or any(unicodedata.category(c).startswith("C") for c in cwd):
        raise ValueError("Не удалось определить каталог панели.")
    text = read_command(
        ["tmux", "capture-pane", "-p", "-J", "-S", f"-{lines}", "-t", pane],
        limit=MAX_BYTES,
    )
    return text, Path(cwd)


def pick(options, title):
    if shutil.which("gum"):
        command = ["gum", "choose", "--header", title]
        result = subprocess.run(
            command,
            input="\n".join(options),
            text=True,
            # gum renders its interactive UI on stderr; only capture the answer.
            stdout=subprocess.PIPE,
            check=False,
        )
    elif shutil.which("fzf"):
        result = subprocess.run(
            ["fzf", "--no-multi", "--prompt", title + ": "],
            check=False,
            input="\n".join(options),
            text=True,
            stdout=subprocess.PIPE,
            env={
                **os.environ,
                "FZF_DEFAULT_OPTS": "",
                "FZF_DEFAULT_OPTS_FILE": "/dev/null",
            },
        )
    else:
        raise ValueError("Нужен gum или fzf для выбора; доступен --list.")
    answer = result.stdout.rstrip("\n")
    return answer if result.returncode == 0 and answer in options else None


def perform(item, action):
    if action in {"copy", "open"} and item["kind"] == "file":
        path = Path(item["path"]).resolve()
        if (
            not path.is_file()
            or any(unicodedata.category(c).startswith("C") for c in str(path))
            or any(unicodedata.category(c).startswith("C") for c in item["value"])
            or any(unicodedata.category(c).startswith("C") for c in item["path"])
            or not isinstance(item["line"], int)
            or item["line"] < 1
        ):
            raise ValueError("Файл недоступен или содержит небезопасные символы.")
    if action == "copy":
        subprocess.run(
            ["pbcopy"],
            input=item["value"],
            text=True,
            capture_output=True,
            timeout=5,
            check=True,
        )
    elif action == "open" and item["kind"] == "url":
        if not safe_url(item["value"]):
            raise ValueError("Небезопасный URL.")
        subprocess.run(
            ["open", item["value"]], capture_output=True, timeout=5, check=True
        )
    elif action == "open" and item["kind"] == "file":
        if not shutil.which("nvim"):
            raise ValueError("nvim не найден.")
        subprocess.run(["nvim", f"+{item['line']}", "--", item["path"]], check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--pane",
        default=os.environ.get("DOTS_SOURCE_PANE") or os.environ.get("TMUX_PANE"),
    )
    parser.add_argument("--lines", type=int, default=200)
    parser.add_argument(
        "--stdin",
        action="store_true",
        help="Читать явно переданный stdin; пути относительно cwd",
    )
    output = parser.add_mutually_exclusive_group()
    output.add_argument("--list", action="store_true")
    output.add_argument("--json", action="store_true")
    args = parser.parse_args()
    try:
        if not 1 <= args.lines <= 5000:
            raise ValueError("lines должен быть в диапазоне 1..5000.")
        if args.stdin:
            raw = sys.stdin.buffer.read(MAX_BYTES + 1)
            if len(raw) > MAX_BYTES:
                raise ValueError("Слишком большой ввод (максимум 1 MiB).")
            text, cwd = raw.decode("utf-8", errors="replace"), Path.cwd()
        else:
            text, cwd = capture(args.pane, args.lines)
        items = extract(text, cwd)
        labels = [
            f"{i + 1}. [{item['kind']}] {item['value']}" for i, item in enumerate(items)
        ]
        if args.json:
            print(json.dumps(items, ensure_ascii=False))
        elif args.list:
            print("\n".join(labels))
        elif not items:
            print("Ссылок не найдено.")
        else:
            selection = pick(labels, "Ссылка из логов")
            if selection is not None:
                item = items[labels.index(selection)]
                actions = ["cancel", "copy"] + (
                    ["open"] if item["kind"] in {"url", "file"} else []
                )
                perform(
                    item, pick(actions, "Действие (open открывает браузер/редактор)")
                )
        return 0
    except KeyboardInterrupt:
        return 130
    except ValueError as exc:
        print(f"dots refs: {exc}", file=sys.stderr)
    except (OSError, subprocess.SubprocessError):
        print(
            "dots refs: внешняя команда недоступна, завершилась с ошибкой или таймаутом.",
            file=sys.stderr,
        )
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
