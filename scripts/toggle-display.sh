#!/bin/bash
STATE=$(hyprctl monitors -j | jq -r '.[] | select(.name == "eDP-1") | .disabled')
if [ "$STATE" == "false" ]; then
	hyprctl keyword monitor "eDP-1,disable"
else
	hyprctl keyword monitor "eDP-1,2560x1600@120,-2560x0,1.25"
	# if pgrep -f hyprpanel >/dev/null; then
	# pkill -f hyprpanel
	# hyprpanel &
	# fi
fi
