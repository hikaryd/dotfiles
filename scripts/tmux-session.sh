#!/usr/bin/env zsh
SESSION_NAME="ghostty"

tmux list-sessions 2>/dev/null
if [ $? -eq 1 ]; then
	exec tmux attach
fi

tmux has-session -t $SESSION_NAME 2>/dev/null
if [ $? -eq 1 ]; then
	exec tmux new-session -d -s $SESSION_NAME
fi

exec tmux attach-session -t $SESSION_NAME
