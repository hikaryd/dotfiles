#!/bin/bash

# Register custom events in sketchybar
sketchybar --add event rift_workspace_changed
sketchybar --add event rift_windows_changed

# Get all workspaces from rift
ALL_WORKSPACES=$(rift-cli query workspaces | jq -r '.[].name' | sort)

for sid in $ALL_WORKSPACES; do
  sketchybar --add item space.$sid left \
    --subscribe space.$sid rift_workspace_changed rift_windows_changed front_app_switched \
    --set space.$sid \
    background.color=0x44ffffff \
    background.corner_radius=5 \
    background.height=20 \
    background.drawing=off \
    icon.padding_left=0 \
    icon.padding_right=0 \
    label.padding_left=3 \
    label.padding_right=3 \
    label.font="sketchybar-app-font:Regular:16.0" \
    click_script="rift-cli execute workspace switch $sid" \
    script="$CONFIG_DIR/plugins/rift.sh $sid"

  if [[ $sid != $(echo "$ALL_WORKSPACES" | tail -n1) ]]; then
    sketchybar --add item space_separator.$sid left \
      --set space_separator.$sid \
      icon="" \
      icon.color=0xffc9c7cd \
      icon.padding_left=8 \
      icon.font="CaskaydiaMono Nerd Font:Regular:20.0" \
      label.drawing=off
  fi
done
