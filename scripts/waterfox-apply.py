#!/usr/bin/env python3
"""Добавить управляемые настройки в использованные профили Waterfox."""

import configparser
import os
import re
import stat
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BEGIN = "// BEGIN dots Waterfox"
END = "// END dots Waterfox"


def profiles(base):
    """Читать реестр профилей, не создавая отсутствующие каталоги."""
    parser = configparser.ConfigParser(interpolation=None)
    parser.read(base / "profiles.ini", encoding="utf-8")
    seen = set()
    for section in parser.sections():
        if not re.fullmatch(r"Profile\d+", section):
            continue
        value = parser.get(section, "Path", fallback="")
        if not value:
            continue
        profile = Path(value)
        if parser.getboolean(section, "IsRelative", fallback=True):
            if profile.is_absolute():
                raise ValueError(
                    f"Относительный профиль содержит абсолютный путь: {section}"
                )
            profile = base / profile
        elif not profile.is_absolute():
            raise ValueError(
                f"Абсолютный профиль содержит относительный путь: {section}"
            )
        profile = profile.resolve()
        if profile not in seen and (profile / "prefs.js").is_file():
            seen.add(profile)
            yield profile


def merge(existing, settings):
    """Заменить только собственный блок, сохранив прочие байты файла."""
    block = f"{BEGIN}\n{settings.rstrip()}\n{END}\n".encode()
    begin, end = BEGIN.encode(), END.encode()
    if begin not in existing and end not in existing:
        separator = b"" if not existing or existing.endswith(b"\n") else b"\n"
        return existing + separator + block
    pattern = re.compile(
        rb"(?m)^" + re.escape(begin) + rb"\r?\n.*?^" + re.escape(end) + rb"(?:\r?\n|$)",
        re.DOTALL,
    )
    if (
        existing.count(begin) != 1
        or existing.count(end) != 1
        or not pattern.search(existing)
    ):
        raise ValueError("Повреждён или дублирован блок dots; файл не изменён")
    return pattern.sub(lambda _: block, existing, count=1)


def apply(profile, settings):
    target = profile / "user.js"
    if target.is_symlink():
        raise ValueError(f"Отказ от изменения симлинка: {target}")
    exists = target.exists()
    original = target.read_bytes() if exists else b""
    updated = merge(original, settings)
    if original == updated:
        return False
    if exists:
        with tempfile.NamedTemporaryFile(
            prefix="user.js.dots-backup-", dir=profile, delete=False
        ) as backup:
            backup.write(original)
    mode = stat.S_IMODE(target.stat().st_mode) if exists else 0o600
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(
            prefix=".user.js.dots-", dir=profile, delete=False
        ) as output:
            temporary = Path(output.name)
            output.write(updated)
            os.fchmod(output.fileno(), mode)
        os.replace(temporary, target)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
    return True


def main():
    base = Path.home() / "Library/Application Support/Waterfox"
    settings = (ROOT / "config/waterfox/user.js").read_text(encoding="utf-8")
    found = list(profiles(base))
    if not found:
        print("Использованных профилей Waterfox нет; пропуск.")
    for profile in found:
        changed = apply(profile, settings)
        print(f"Waterfox: {profile.name}: {'обновлён' if changed else 'без изменений'}")


if __name__ == "__main__":
    main()
