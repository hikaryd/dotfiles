#!/bin/sh
# tmux 3.7–3.7c проверяет перекрытие popup без смещения верхнего status.
# На время popup переносим status вниз: размеры панелей не меняются.
set -eu
client=$1
session=$2
shift 2

show_popup() {
    result=0
    tmux display-popup -c "$client" "$@" || result=$?
    # Отмена popup (SIGHUP) не должна открывать view-mode ошибкой run-shell.
    [ "$result" -eq 129 ] && return 0
    return "$result"
}

case "$(tmux display-message -p '#{version}')" in
    3.7|3.7a|3.7b|3.7c) ;;
    *) show_popup "$@"; exit ;;
esac

# Один управляемый popup на сессию: два клиента не восстанавливают status
# друг другу посреди popup. Lock удерживается лишь до закрытия/ошибки.
lock="dots-popup-$session"
tmux wait-for -L "$lock"
changed=0
original=
cleanup() {
    if [ "$changed" = 1 ]; then
        # Не перезаписываем настройку, явно изменённую пользователем за это время.
        if [ "$(tmux show-options -qv -t "$session" status-position)" = bottom ]; then
            if [ -n "$original" ]; then
                tmux set-option -t "$session" status-position "$original" || :
            else
                tmux set-option -u -t "$session" status-position || :
            fi
        fi
    fi
    tmux wait-for -U "$lock" || :
}
trap 'cleanup' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP
# -A marks inherited options with '*': one probe gives both value and scope.
position=$(tmux show-options -Aq -t "$session" status-position)
case "$position" in
    'status-position top'|'status-position* top')
        if [ "$position" = 'status-position top' ]; then
            original=top
        fi
        changed=1
        tmux set-option -t "$session" status-position bottom
        ;;
esac
show_popup "$@"
