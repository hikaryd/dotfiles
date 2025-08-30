#!/usr/bin/env bash
set -Eeuo pipefail

N="${1:-}"
[[ "$N" =~ ^[0-9]$ ]] || {
	echo "usage: move-to-space.sh <0-9>"
	exit 2
}

# Кейкоды для цифр
case "$N" in
1) KC=18 ;; 2) KC=19 ;; 3) KC=20 ;; 4) KC=21 ;;
5) KC=23 ;; 6) KC=22 ;; 7) KC=26 ;; 8) KC=28 ;; 9) KC=25 ;;
0) KC=29 ;;
esac

# Координата «захвата» в тайтлбаре активного окна (чуть правее/ниже левого верхнего угла)
WJSON="$(yabai -m query --windows --window 2>/dev/null || true)"
[[ -n "$WJSON" ]] || exit 1
X=$(jq -r '.frame.x + 20' <<<"$WJSON")
Y=$(jq -r '.frame.y + 14' <<<"$WJSON")

# Запомним позицию курсора, чтобы вернуть после
POS="$(cliclick p 2>/dev/null || true)" # формат "x,y"
MX="${POS%%,*}"
MY="${POS##*,}"

# Начинаем drag: навести → зажать → чуть сдвинуть
cliclick m:${X},${Y} dd:${X},${Y} dm:+3,+3 >/dev/null 2>&1
sleep 0.08

# Переключаемся на нужный Desktop (как в Mission Control: Option+цифра)
osascript -e 'tell application "System Events" to key down option' \
	-e "tell application \"System Events\" to key code $KC" \
	-e 'tell application "System Events" to key up option' >/dev/null

# Даём системе переключиться и «донести» окно
sleep 0.12

# Отпускаем мышь и возвращаем курсор
cliclick du:. >/dev/null 2>&1
[[ -n "$MX" && -n "$MY" ]] && cliclick m:${MX},${MY} >/dev/null 2>&1
