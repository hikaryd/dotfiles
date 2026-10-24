#!/usr/bin/env bash

set -euo pipefail

readonly DOMAIN='com.vorssaint.utils'

# A running app can overwrite externally applied preferences when it exits.
# Stop it before writing, and restore it only after all preferences are saved.
was_running=false
if pgrep -x Vorssaint >/dev/null; then
	was_running=true
	osascript -e 'tell application "Vorssaint" to quit'
	for ((attempt = 0; attempt < 50; attempt++)); do
		pgrep -x Vorssaint >/dev/null || break
		sleep 0.1
	done
	if pgrep -x Vorssaint >/dev/null; then
		echo 'Vorssaint did not quit; preferences were not changed.' >&2
		exit 1
	fi
fi

# Keep Vorssaint focused on the two jobs it replaces on this machine. These
# keys are the stable feature identifiers published by Vorssaint's feature hub.
for feature in mixer soundOutputSwitcher micMute musicBlock monitorPower; do
	defaults write "$DOMAIN" "featureAvailable.$feature" -bool true
done

# Power replaces AirStats: show the useful battery readings directly in the
# menu bar while retaining the full Power panel in Vorssaint.
defaults write "$DOMAIN" menuBarBattery -bool true
defaults write "$DOMAIN" menuBarBatteryTime -bool true
defaults write "$DOMAIN" menuBarBatteryTemperature -bool true
defaults write "$DOMAIN" menuBarPeripheralBattery -bool true

# Sound replaces SoundSource. Per-app mixing is available from the panel; the
# remaining switches enable output cycling, mic mute, and Music launch blocking.
defaults write "$DOMAIN" soundOutputSwitcherEnabled -bool true
defaults write "$DOMAIN" micMuteShortcutEnabled -bool true
defaults write "$DOMAIN" musicBlockEnabled -bool true

# The app reconciles this preference with its ServiceManagement login item on
# launch, so the replacement survives logout and reboot.
defaults write "$DOMAIN" launchAtLoginWanted -bool true

if "$was_running"; then
	open -a Vorssaint
fi

echo 'Vorssaint battery and sound defaults configured'
