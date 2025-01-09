#!/bin/bash

linux_notify() {
	echo -e "\007"
	echo "🎧 AirPods Max Update: $1"
}

terminal_notify() {
	notify-send "🎧 AirPods" "$1"
}

macos_notify() {
	osascript -e "display notification \"$1\" with title \"AirPods Max\""
}

if [[ "$OSTYPE" == "darwin"* ]]; then
	notify() {
		macos_notify "$1"
	}
else
	notify() {
		terminal_notify "$1"
		linux_notify "$1"
	}
fi

LAST_BATTERY=-1
LAST_CHARGING="false"

while true; do
	AIRPODS_DATA=$(python3 $HOME/dotfiles/scripts/airpods.py --json)

	if [ $? -ne 0 ]; then
		notify "Error reading AirPods data"
		sleep 30
		continue
	fi

	STATUS=$(echo $AIRPODS_DATA | jq -r '.status')
	if [ "$STATUS" = "0" ]; then
		sleep 30
		continue
	fi

	CURRENT_BATTERY=$(echo $AIRPODS_DATA | jq -r '.charge.left')
	CURRENT_CHARGING=$(echo $AIRPODS_DATA | jq -r '.charging_left')

	if [ "$CURRENT_BATTERY" != "$LAST_BATTERY" ] || [ "$CURRENT_CHARGING" != "$LAST_CHARGING" ]; then
		MESSAGE="Battery: ${CURRENT_BATTERY}%"
		if [ "$CURRENT_CHARGING" = "true" ]; then
			MESSAGE="$MESSAGE (Charging)"
		fi

		notify "$MESSAGE"

		LAST_BATTERY=$CURRENT_BATTERY
		LAST_CHARGING=$CURRENT_CHARGING
	fi

	sleep 30
done
