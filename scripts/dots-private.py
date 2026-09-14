#!/usr/bin/env python3
"""Подготовка и локальная проверка приватных настроек; секреты не исполняются."""

import argparse
import os
import re
import stat
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GIT_TEMPLATE = """# Личная Git-идентичность; раскомментируйте и заполните.
# [user]
#     name = ВАШЕ_ИМЯ
#     email = ВАША_ПОЧТА
# Репозитории обслуживания можно добавить секцией [maintenance], ключ repo.
"""


def locations():
    home = Path.home()
    xdg = Path(os.environ.get("XDG_CONFIG_HOME") or home / ".config")
    codex = Path(
        os.environ.get("CODEX_CONFIG_DIR")
        or os.environ.get("CODEX_HOME")
        or home / ".codex"
    )
    claude = Path(os.environ.get("CLAUDE_CONFIG_DIR") or home / ".claude")
    return [
        (
            "shell",
            Path(os.environ.get("DOTS_PRIVATE_ZSH") or xdg / "dots/private.zsh"),
            "config/zsh/private.example.zsh",
        ),
        ("git", home / ".config/git/private.inc", None),
        ("claude", claude / ".mcp-secrets", "config/claude/mcp-secrets.example.sh"),
        ("codex", codex / ".mcp-secrets", "config/codex/mcp-secrets.example.sh"),
    ]


def validate_path(path):
    if not path.is_absolute():
        raise ValueError("нужен абсолютный путь")
    if path.is_symlink():
        raise ValueError("символическая ссылка: проверьте вручную")
    if path.resolve().is_relative_to(ROOT):
        raise ValueError("приватный файл не может находиться в репозитории")
    if path.exists() and not path.is_file():
        raise ValueError("ожидался обычный файл")


def check(path):
    # O_NOFOLLOW исключает чтение ссылки, появившейся после проверки пути.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd) as handle:
        info = os.fstat(handle.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
            raise ValueError("нужен обычный файл текущего пользователя")
        if stat.S_IMODE(info.st_mode) != 0o600:
            raise ValueError("установите права 600; автоматически не меняю")
        active = "\n".join(
            line
            for line in handle.read().splitlines()
            if line.strip() and not line.lstrip().startswith("#")
        )
    if not active or re.search(
        r"your-|example-|\b(?:dev|stage|preprod|prod)-context\b|ВАШ[АЕ]", active
    ):
        raise ValueError("шаблон не заполнен или остались примеры")
    if not any(
        re.search(r"=[ \t]*[^ \t'\"]|=[ \t]*['\"][^'\"]", line)
        for line in active.splitlines()
    ):
        raise ValueError("нет заполненных значений")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("init", "check"), nargs="?", default="check")
    args = parser.parse_args()
    failed = False
    entries = locations()
    # Проверяем все пути до первого создания: ошибочный override не даёт частичный init.
    for label, path, _ in entries:
        try:
            validate_path(path)
        except (ValueError, OSError, RuntimeError) as error:
            print(
                f"{label}: {error if isinstance(error, ValueError) else 'недоступный путь'}"
            )
            failed = True
    if failed:
        return 1
    for label, path, template in entries:
        display = str(path).replace(str(Path.home()), "~", 1)
        try:
            if args.action == "init":
                path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
                try:
                    fd = os.open(
                        path,
                        os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
                        0o600,
                    )
                except FileExistsError:
                    print(f"{label}: сохранён существующий файл ({display})")
                    continue
                with os.fdopen(fd, "w") as handle:
                    handle.write(
                        (ROOT / template).read_text() if template else GIT_TEMPLATE
                    )
                print(f"{label}: создан шаблон, заполните ({display})")
            else:
                check(path)
                print(
                    f"{label}: права и базовая проверка заполнения в порядке ({display})"
                )
        except (OSError, ValueError) as error:
            message = (
                str(error)
                if isinstance(error, ValueError) and not isinstance(error, UnicodeError)
                else "файл отсутствует, недоступен или не является текстом"
            )
            print(f"{label}: {message} ({display})")
            failed = True
    if args.action == "check":
        print(
            "Значения не выводятся и не исполняются. Доступность сервисов и действительность ключей не проверяются."
        )
    return int(failed)


if __name__ == "__main__":
    raise SystemExit(main())
