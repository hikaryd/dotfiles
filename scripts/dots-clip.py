#!/usr/bin/env python3
"""Локальные преобразования clipboard; запись только явно, без файлов и сети."""

import argparse
import base64
import binascii
import datetime as dt
import json
import math
import os
import re
import selectors
import shutil
import subprocess
import sys
import time
import unicodedata
from typing import Any

MAX_BYTES = 1024 * 1024
MODES = ("json", "compact", "base64", "jwt", "time", "redact")
DEFAULT_KEYS = ("token", "secret", "password", "authorization", "api_key")


class ClipError(Exception):
    pass


def checked_text(text: str) -> str:
    if any(
        unicodedata.category(c) in ("Cc", "Cf", "Cs") and c not in "\n\r\t"
        for c in text
    ):
        raise ClipError(
            "Управляющие символы или некорректный Unicode: операция отменена."
        )
    if len(text.encode("utf-8")) > MAX_BYTES:
        raise ClipError("Лимит 1 MiB превышен.")
    return text


def decode_input(data: bytes) -> str:
    if len(data) > MAX_BYTES:
        raise ClipError("Лимит 1 MiB превышен.")
    try:
        return checked_text(data.decode("utf-8"))
    except UnicodeError:
        raise ClipError("Ожидался корректный UTF-8.") from None


def strict_json(text: str) -> Any:
    def constant(_: str) -> Any:
        raise ValueError

    def pairs(items: list[tuple[str, Any]]) -> dict[str, Any]:
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError
            result[key] = value
        return result

    value = json.loads(text, parse_constant=constant, object_pairs_hook=pairs)

    def validate(item: Any) -> None:
        if isinstance(item, str):
            checked_text(item)
        elif isinstance(item, dict):
            for key, child in item.items():
                checked_text(key)
                validate(child)
        elif isinstance(item, list):
            for child in item:
                validate(child)

    validate(value)
    return value


def redact(value: Any, keys: set[str]) -> Any:
    if isinstance(value, dict):
        return {
            k: "[REDACTED]" if k.casefold() in keys else redact(v, keys)
            for k, v in value.items()
        }
    if isinstance(value, list):
        return [redact(v, keys) for v in value]
    return value


def decode_segment(segment: str) -> bytes:
    if not re.fullmatch(r"[A-Za-z0-9_-]+", segment):
        raise ValueError
    data = base64.b64decode(
        segment + "=" * (-len(segment) % 4), altchars=b"-_", validate=True
    )
    if base64.urlsafe_b64encode(data).decode().rstrip("=") != segment:
        raise ValueError
    return data


def transform(mode: str, source: str, keys: list[str] | None = None) -> str:
    try:
        checked_text(source)
        if mode == "base64":
            encoded = source.strip()
            decoded = base64.b64decode(encoded, validate=True)
            if base64.b64encode(decoded).decode() != encoded:
                raise ValueError
            return decode_input(decoded)
        if mode in ("json", "compact", "redact"):
            value = strict_json(source)
            if mode == "redact":
                value = redact(
                    value, {k.casefold() for k in (keys or list(DEFAULT_KEYS))}
                )
        elif mode == "jwt":
            header, payload, signature = source.strip().split(".")
            decode_segment(signature)
            header_value = strict_json(decode_input(decode_segment(header)))
            payload_value = strict_json(decode_input(decode_segment(payload)))
            if not isinstance(header_value, dict) or not isinstance(
                payload_value, dict
            ):
                raise ValueError
            if (
                not isinstance(header_value.get("alg"), str)
                or header_value["alg"].lower() == "none"
            ):
                raise ValueError
            value = {
                "verification": "UNVERIFIED",
                "header": header_value,
                "payload": payload_value,
            }
        elif mode == "time":
            text = source.strip()
            if re.fullmatch(r"-?\d+(?:\.\d{1,6})?", text):
                seconds = float(text)
                # Современные 13-значные epoch values трактуются как миллисекунды.
                if abs(seconds) >= 100_000_000_000:
                    seconds /= 1000
                moment = dt.datetime.fromtimestamp(seconds, tz=dt.timezone.utc)
            else:
                if not re.fullmatch(
                    r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)",
                    text,
                ):
                    raise ValueError
                moment = dt.datetime.fromisoformat(text).astimezone(dt.timezone.utc)
                seconds = moment.timestamp()
            if not math.isfinite(seconds):
                raise ValueError
            value = {
                "iso8601": moment.isoformat().replace("+00:00", "Z"),
                "unix_seconds": seconds,
                "unix_milliseconds": round(seconds * 1000),
            }
        else:
            raise ValueError
        result = json.dumps(
            value,
            ensure_ascii=False,
            allow_nan=False,
            indent=None if mode == "compact" else 2,
            separators=(",", ":") if mode == "compact" else None,
        )
        return checked_text(result)
    except (
        ValueError,
        UnicodeError,
        binascii.Error,
        OverflowError,
        RecursionError,
        OSError,
    ):
        raise ClipError(
            "Некорректные данные для выбранного преобразования; clipboard не изменён."
        ) from None


def preview(text: str) -> str:
    # ASCII escapes keep OSC, bidi and unusual Unicode out of terminal control paths.
    escaped = json.dumps(text[:2000], ensure_ascii=True)[1:-1]
    return escaped + (" … [preview truncated]" if len(text) > 2000 else "")


def read_stdin() -> bytes:
    return sys.stdin.buffer.read(MAX_BYTES + 1)


def read_clipboard() -> bytes:
    process = subprocess.Popen(
        ["pbpaste"], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL
    )
    assert process.stdout is not None
    try:
        chunks = bytearray()
        deadline = time.monotonic() + 5
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not selector.select(remaining):
                    raise ClipError("Чтение clipboard превысило таймаут.")
                part = os.read(
                    process.stdout.fileno(), min(65536, MAX_BYTES + 1 - len(chunks))
                )
                chunks.extend(part)
                if len(chunks) > MAX_BYTES:
                    raise ClipError("Лимит 1 MiB превышен.")
                if not part:
                    break
        if process.wait(timeout=max(0.01, deadline - time.monotonic())):
            raise ClipError("Не удалось прочитать clipboard.")
        return bytes(chunks)
    finally:
        if process.poll() is None:
            process.kill()
        process.wait()
        process.stdout.close()


def change_count() -> int:
    """Read only native pasteboard generation; no content crosses osascript."""
    result = subprocess.run(
        [
            "/usr/bin/osascript",
            "-l",
            "JavaScript",
            "-e",
            "ObjC.import('AppKit'); $.NSPasteboard.generalPasteboard.changeCount",
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        timeout=5,
        check=False,
    )
    value = result.stdout.strip()
    if result.returncode or not re.fullmatch(r"[0-9]{1,20}", value):
        raise ClipError("Недоступен changeCount clipboard: запись отменена.")
    return int(value)


def clipboard_snapshot(expected_count: int | None = None) -> tuple[bytes, int]:
    before = change_count()
    if expected_count is not None and before != expected_count:
        raise ClipError("Clipboard изменился после чтения: запись отменена.")
    data = read_clipboard()
    if change_count() != before:
        raise ClipError("Clipboard изменился во время чтения: запись отменена.")
    return data, before


def write_clipboard(data: bytes) -> None:
    result = subprocess.run(
        ["pbcopy"],
        input=data,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        timeout=5,
        check=False,
    )
    if result.returncode:
        raise ClipError("pbcopy сообщил об ошибке; состояние clipboard неизвестно.")


def choose_mode() -> str | None:
    if shutil.which("gum"):
        result = subprocess.run(
            ["gum", "choose", *MODES], stdout=subprocess.PIPE, text=True, check=False
        )
    elif shutil.which("fzf"):
        result = subprocess.run(
            ["fzf", "--prompt=Clipboard transform > "],
            input="\n".join(MODES),
            stdout=subprocess.PIPE,
            text=True,
            check=False,
        )
    else:
        raise ClipError("Укажите подкоманду: json, compact, base64, jwt, time, redact.")
    selected = result.stdout.strip()
    return selected if result.returncode == 0 and selected in MODES else None


def confirm() -> bool:
    try:
        # Separate handles avoid BufferedRandom's seek requirement on a PTY.
        with open("/dev/tty", "w", encoding="utf-8") as prompt:
            prompt.write("Перезаписать clipboard? Введите yes: ")
            prompt.flush()
        with open("/dev/tty", encoding="utf-8") as answer:
            return answer.readline(64).strip() == "yes"
    except OSError:
        return False


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=MODES, nargs="?")
    action = parser.add_mutually_exclusive_group()
    action.add_argument(
        "--stdout", action="store_true", help="Только stdout, без записи clipboard"
    )
    action.add_argument(
        "--write", action="store_true", help="Явная запись без подтверждения"
    )
    parser.add_argument(
        "--stdin", action="store_true", help="Вход из stdin вместо clipboard"
    )
    parser.add_argument(
        "--keys", help="redact: точные ключи через запятую, без учёта регистра"
    )
    args = parser.parse_args(argv)
    try:
        if args.keys is not None and (
            args.mode != "redact" or not all(k.strip() for k in args.keys.split(","))
        ):
            raise ClipError("--keys допустим только для redact и непустых имён.")
        mode = args.mode or choose_mode()
        if mode is None:
            return 130
        # stdin/stdout path never touches the OS pasteboard.
        generation = None
        if args.stdout:
            original = None if args.stdin else read_clipboard()
        else:
            original, generation = clipboard_snapshot()
        source = read_stdin() if args.stdin else original
        assert source is not None
        result = transform(
            mode,
            decode_input(source),
            [k.strip() for k in args.keys.split(",")] if args.keys else None,
        )
        if args.stdout:
            print(result)
            return 0
        if mode == "jwt":
            print("JWT UNVERIFIED: подпись и доверие НЕ проверены.", file=sys.stderr)
        if mode == "redact":
            print(
                "Redact: только указанные ключи; это не гарантия удаления всех секретов.",
                file=sys.stderr,
            )
        if not args.write:
            print(preview(result), file=sys.stderr)
            if not confirm():
                return 130
        # changeCount catches same-text/rich-content changes and ABA; the final
        # compare/copy gap remains best effort because pbcopy is not a CAS API.
        current, _ = clipboard_snapshot(generation)
        if current != original:
            raise ClipError("Clipboard изменился после чтения: запись отменена.")
        write_clipboard(result.encode("utf-8"))
        print("Clipboard обновлён.", file=sys.stderr)
        return 0
    except (ClipError, OSError, subprocess.SubprocessError) as error:
        print(
            str(error)
            if isinstance(error, ClipError)
            else "Ошибка локального инструмента clipboard.",
            file=sys.stderr,
        )
        return 1
    except (KeyboardInterrupt, EOFError):
        return 130


if __name__ == "__main__":
    raise SystemExit(main())
