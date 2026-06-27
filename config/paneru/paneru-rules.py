#!/usr/bin/env python3
"""paneru-rules — базовый app -> virtual workspace авто-пиннинг для paneru.

paneru НЕ умеет привязывать приложения к рядам в своём конфиге (в [windows.*]
есть только floating/index/width/grid). Поэтому привязку, как в rift, делаем
снаружи: слушаем поток событий `paneru subscribe --json` и, когда НОВОЕ окно
известного приложения впервые получает фокус, отправляем его в «домашний» ряд
командой `paneru send-cmd window virtualmovenum N`.

Принципы (важно для предсказуемости):
  * Окно авто-раскладывается РОВНО ОДИН РАЗ — при первом появлении его
    window_id. Дальше его можно двигать руками — демон не вернёт назад.
  * На старте (и после рестарта демона/paneru) все уже существующие окна
    помечаются как «уже разложенные», поэтому текущая раскладка не ломается —
    переедут только реально новые окна.
  * Формат события (envelope) у paneru не зафиксирован в публичных доках,
    поэтому парсер ТОЛЕРАНТНЫЙ: ищет window_id / bundle_id /
    virtual_workspace_number (и их focused_*-варианты) на любой глубине JSON.
    При PANERU_RULES_DEBUG=1 первые сырые события пишутся в лог — это поможет
    свериться и подправить при первом запуске на живом paneru.

Запуск вручную (для теста, в foreground, лог в stderr):
    PANERU_RULES_DEBUG=1 python3 ~/.config/paneru/paneru-rules.py

Как сервис — через LaunchAgent config/paneru/com.paneru.rules.plist.
"""

from __future__ import annotations  # аннотации как строки -> работает и на Python 3.9

import json
import os
import shutil
import subprocess
import sys
import time
from typing import Any

# ─────────────────────────────────────────────────────────────────────────
# Карта: bundle_id -> номер ряда (virtual workspace).
# Номера рядов = window_virtualnum_N в paneru.toml
# (ряд 1=Alt+1, 2=Alt+3, 3=Alt+W, 4=Alt+E, 5=Alt+D).
# Floating-приложения (1Password, Spotify, Raycast, Finder, ...) сюда НЕ
# вносим — они не тайлятся и ряда не имеют.
# bundle_id установленного приложения:
#   /usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "/Applications/<App>.app/Contents/Info.plist"
# ─────────────────────────────────────────────────────────────────────────
RULES = {
    # ── Ряд 1 (Alt+1) — Браузеры + Толк + Музыка ─────────────────────────
    "org.chromium.Thorium": 1,
    "app.zen-browser.zen": 1,
    "org.mozilla.firefox": 1,
    "com.apple.Safari": 1,
    "kontur.talk": 1,                        # Толк
    "com.apple.Music": 1,                    # Музыка
    # "company.thebrowser.Browser": 1,       # Arc — раскомментируй, если стоит
    # "com.brave.Browser": 1,                # Brave
    # TODO bundle_id: Ora, Comet, Helium

    # ── Ряд 2 (Alt+3) — Мессенджеры / почта ──────────────────────────────
    "one.ayugram.AyuGramDesktop": 2,         # AyuGram
    "ru.unlimitedtech.express.desktop": 2,   # X5_Rooms
    # "org.telegram.desktop": 2,             # Telegram — если поставишь офиц. клиент
    # "ch.protonmail.desktop": 2,            # Proton Mail
    # TODO bundle_id: Psst

    # ── Ряд 3 (Alt+W) — Документы + Outlook ──────────────────────────────
    "com.microsoft.Outlook": 3,
    "com.getupnote.desktop": 3,              # UpNote
    "com.apple.iWork.Pages": 3,
    "com.apple.Preview": 3,                  # Просмотр

    # ── Ряд 4 (Alt+E) — Система ──────────────────────────────────────────
    "com.apple.systempreferences": 4,        # System Settings
    "com.apple.AppStore": 4,

    # ── Ряд 5 (Alt+D) — Терминал + дев/БД ────────────────────────────────
    "com.mitchellh.ghostty": 5,
    "com.github.wez.wezterm": 5,
    "org.alacritty": 5,
    "com.electron.dockerdesktop": 5,         # Docker Desktop (GUI)
    "com.docker.docker": 5,
    "org.jkiss.dbeaver.core.product": 5,     # DBeaver
    "com.kubeli": 5,                         # Kubeli
    "com.electron.kontena-lens": 5,          # Lens
    "io.beekeeperstudio.desktop": 5,         # Beekeeper
    "at.eggerapps.Postico": 5,               # Postico
}

# True  -> переехать вместе с окном в его ряд (virtualmovenum): «я открыл — покажи».
# False -> тихо подшить окно в его ряд, остаться на месте (virtualsendnum).
FOLLOW = True

# Сколько секунд ждать перед переподключением, если демон/сокет недоступен.
RECONNECT_DELAY = 3.0

# Прайминг существующих окон: paneru восстанавливает сессию не мгновенно после
# старта, поэтому повторяем query, пока не увидим окна (или не выйдет таймаут).
PRIME_ATTEMPTS = 10
PRIME_RETRY_DELAY = 0.5

DEBUG = os.environ.get("PANERU_RULES_DEBUG") == "1"
_debug_budget = 25  # сколько первых сырых событий вывалить в лог при DEBUG


def log(*args: object) -> None:
    print("[paneru-rules]", *args, file=sys.stderr, flush=True)


def find_paneru() -> str | None:
    """Найти бинарь paneru: PATH, brew, cargo."""
    candidates = [
        "paneru",
        "/opt/homebrew/bin/paneru",
        os.path.expanduser("~/.cargo/bin/paneru"),
        "/usr/local/bin/paneru",
    ]
    for c in candidates:
        if "/" in c:
            if os.path.exists(c):
                return c
        else:
            found = shutil.which(c)
            if found:
                return found
    return None


def deep_get(obj: Any, *keys: str) -> Any:
    """Рекурсивно найти первое НЕ-контейнерное значение по любому из ключей."""
    if isinstance(obj, dict):
        for k in keys:
            if k in obj and not isinstance(obj[k], (dict, list)):
                return obj[k]
        for v in obj.values():
            r = deep_get(v, *keys)
            if r is not None:
                return r
    elif isinstance(obj, list):
        for v in obj:
            r = deep_get(v, *keys)
            if r is not None:
                return r
    return None


def send_move(paneru: str, n: int) -> None:
    cmd = "virtualmovenum" if FOLLOW else "virtualsendnum"
    try:
        subprocess.run(
            [paneru, "send-cmd", "window", cmd, str(n)],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=5,
        )
    except Exception as e:  # noqa: BLE001
        log("send-cmd failed:", e)


def prime_seen(paneru: str) -> set[Any]:
    """Пометить все существующие окна как уже разложенные (не трогаем их).

    Повторяем запрос, пока paneru не вернёт окна (или не выйдет таймаут): иначе
    при старте в момент рестарта paneru сессия ещё пуста, мы «не запомним»
    существующие окна и потом дёрнем их на домашние ряды при первом фокусе.
    """
    seen: set[Any] = set()
    for _ in range(PRIME_ATTEMPTS):
        try:
            out = subprocess.run(
                [paneru, "query", "state", "--json"],
                capture_output=True,
                text=True,
                timeout=5,
            )
            data = json.loads(out.stdout or "{}")
            seen = {
                w.get("window_id")
                for vw in (data.get("virtual_workspaces") or [])
                for w in (vw.get("windows") or [])
                if w.get("window_id") is not None
            }
        except Exception as e:  # noqa: BLE001
            log("prime_seen query failed (paneru not ready?):", e)
        if seen:
            break
        time.sleep(PRIME_RETRY_DELAY)
    log(f"primed {len(seen)} existing window(s)")
    return seen


def watch(paneru: str) -> None:
    global _debug_budget
    seen = prime_seen(paneru)

    proc = subprocess.Popen(
        [paneru, "subscribe", "--json"],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        bufsize=1,
    )
    log("subscribed; watching for new windows")
    assert proc.stdout is not None
    for line in proc.stdout:
        line = line.strip()
        if not line:
            continue
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue

        if DEBUG and _debug_budget > 0:
            log("raw:", line[:400])
            _debug_budget -= 1

        wid = deep_get(ev, "window_id", "focused_window_id")
        bundle = deep_get(ev, "bundle_id", "focused_bundle_id")
        vws = deep_get(ev, "virtual_workspace_number")

        if wid is None or wid in seen:
            continue
        # Первое появление этого окна — фиксируем, чтобы тронуть максимум раз.
        seen.add(wid)

        if not bundle:
            continue
        target = RULES.get(bundle)
        if target is None:
            continue
        if vws == target:
            continue  # уже дома

        log(f"placing window {wid} ({bundle}) -> row {target}")
        send_move(paneru, target)

    # subscribe завершился — демон, видимо, остановлен/перезапущен.
    proc.wait()
    log("subscribe stream ended")


def main() -> None:
    paneru = find_paneru()
    if not paneru:
        log("paneru binary not found in PATH/brew/cargo; retrying...")
    while True:
        try:
            if paneru is None:
                paneru = find_paneru()
            if paneru is None:
                time.sleep(RECONNECT_DELAY)
                continue
            watch(paneru)
        except KeyboardInterrupt:
            log("interrupted, exiting")
            return
        except Exception as e:  # noqa: BLE001
            log("watch error:", e)
        time.sleep(RECONNECT_DELAY)


if __name__ == "__main__":
    main()
