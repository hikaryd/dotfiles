#!/bin/sh
# One persistent player; popups attach a client, never own the player process.
set -eu
self_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
player_session=dots-spotatui

quote() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

# Explicit non-interactive shell: no completion/prompt/OMX startup overhead.
if [ "${1:-}" = --run ]; then
    exec /bin/bash --noprofile --norc -c '
        source "$HOME/.config/shell/codex-proxy.sh" || exit
        spotatui --no-update
    '
fi

# The server lock serializes simultaneous first opens, but not popup lifetimes.
lock=dots-spotatui-launch
locked=0
placeholder=
cleanup() {
    if [ -n "$placeholder" ]; then
        tmux kill-window -t "$placeholder" 2>/dev/null || :
    fi
    if [ "$locked" = 1 ]; then tmux wait-for -U "$lock" || :; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

if ! tmux has-session -t "=$player_session" 2>/dev/null; then
    tmux wait-for -L "$lock"
    locked=1
    # Another first-open caller may have created it while we waited.
    if ! tmux has-session -t "=$player_session" 2>/dev/null; then
        # Link an existing single-pane player window without moving its pane.
        # Refuse ambiguous/multi-pane cases rather than start a second player.
        players=$(tmux list-panes -a -F '#{pane_current_command}|#{window_id}|#{window_panes}' | awk -F '|' '$1 == "spotatui" { if (!seen[$2]++) print $2 "|" $3 }')
        if [ -n "$players" ]; then
            if [ "$(printf '%s\n' "$players" | wc -l | tr -d ' ')" != 1 ] || [ "${players##*|}" != 1 ]; then
                echo 'Spotatui is already running in multiple windows or a split window; not starting another player.' >&2
                exit 1
            fi
            window=${players%|*}
            placeholder=$(tmux new-session -d -s "$player_session" -P -F '#{window_id}' /bin/sleep 86400)
            tmux link-window -s "$window" -t "$player_session:"
            tmux kill-window -t "$placeholder"
            placeholder=
        else
            if pgrep -x spotatui >/dev/null 2>&1; then
                echo 'Spotatui is running outside this tmux server; not starting another player.' >&2
                exit 1
            fi
            tmux new-session -d -s "$player_session" -c "$HOME" "$(quote "$self_dir/spotatui-popup.sh") --run"
        fi
        tmux set-option -t "$player_session" status off
        # Last popup detaching must not destroy the player.
        tmux set-option -t "$player_session" destroy-unattached off
    fi
    tmux wait-for -U "$lock"
    locked=0
fi

socket=$(tmux display-message -p '#{socket_path}')
attach="exec env -u TMUX tmux -S $(quote "$socket") attach-session -t $(quote "=$player_session")"
if [ "$#" -eq 2 ]; then
    client=$1
    session=$2
elif [ -n "${TMUX:-}" ]; then
    client=$(tmux display-message -p '#{client_tty}')
    session=$(tmux display-message -p '#{session_id}')
else
    exec env -u TMUX tmux -S "$socket" attach-session -t "=$player_session"
fi
# A linked window cannot display a popup containing itself.
current_window=$(tmux display-message -p -t "$session:" '#{window_id}')
if tmux list-windows -t "=$player_session" -F '#{window_id}' | grep -Fxq "$current_window"; then
    tmux display-message -c "$client" 'Spotatui is already in this window'
    exit 0
fi
"$self_dir/popup.sh" "$client" "$session" -E -w 90% -h 85% -T ' Spotify · detach: Ctrl-a d ' "$attach"
