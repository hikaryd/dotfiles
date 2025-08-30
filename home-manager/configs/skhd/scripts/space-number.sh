#!/usr/bin/env bash
set -Eeuo pipefail

N="${1:-}"
[[ "$N" =~ ^[0-9]$ ]] || {
	echo "usage: space-number.sh <0-9>"
	exit 2
}

# Маппинг Apple key codes для цифр:
# 1..4 = 18 19 20 21; 5..9 = 23 22 26 28 25; 0 = 29
case "$N" in
1) KC=18 ;; 2) KC=19 ;; 3) KC=20 ;; 4) KC=21 ;;
5) KC=23 ;; 6) KC=22 ;; 7) KC=26 ;; 8) KC=28 ;; 9) KC=25 ;;
0) KC=29 ;;
esac

mkdir -p "$HOME/.cache/yabai"

# Сохраняем предыдущий спейс (если получится)
CUR="$(yabai -m query --spaces --space 2>/dev/null | jq -r '.index' 2>/dev/null || true)"
[[ -n "$CUR" && "$CUR" != "null" ]] && printf '%s\n' "$CUR" >"$HOME/.cache/yabai/prev_space"

# Жмём Option+<цифра>
osascript -e 'tell application "System Events" to key down option' \
	-e "tell application \"System Events\" to key code $KC" \
	-e 'tell application "System Events" to key up option' >/dev/null

# Обновляем current_space (не критично, но полезно)
sleep 0.15
NEW="$(yabai -m query --spaces --space 2>/dev/null | jq -r '.index' 2>/dev/null || true)"
[[ -n "$NEW" && "$NEW" != "null" ]] && printf '%s\n' "$NEW" >"$HOME/.cache/yabai/current_space"
