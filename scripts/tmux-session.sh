#!/usr/bin/env zsh
SESSION_NAME="ghostty"

if tmux has-session -t "$SESSION_NAME" >/dev/null 2>&1; then
	exec tmux attach-session -t $SESSION_NAME
else
	exec tmux attach
fi
