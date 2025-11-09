#!/usr/bin/env bash

# Ensure it's executable:
# chmod +x ~/.config/sketchybar/plugins/rift.sh

# Get the focused workspace from rift
WORKSPACES_JSON=$(rift-cli query workspaces)
FOCUSED_WORKSPACE=$(echo "$WORKSPACES_JSON" | jq -r '.[] | select(.is_active == true) | .name')

update_workspace_icon() {
    local workspace_name=$1

    # Get workspace info and extract bundle_ids
    local workspace_info=$(echo "$WORKSPACES_JSON" | jq -r ".[] | select(.name == \"$workspace_name\")")

    # Extract bundle_ids and map them to icons using the icon_map_fn
    local APP_ICONS=""
    while IFS= read -r bundle_id; do
        if [ -z "$bundle_id" ]; then
            continue
        fi

        # Use the icon_map_fn.sh with bundle_id
        local icon=$("$CONFIG_DIR/plugins/icon_map_fn.sh" "$bundle_id")

        # If icon is still default, try to extract app name from bundle_id
        if [ "$icon" == ":default:" ]; then
            # For bundle IDs like "com.spotify.client" or just "Spotify"
            # Extract the last component or use as-is
            local app_name="$bundle_id"
            if [[ "$bundle_id" == *"."* ]]; then
                # Extract last part after last dot and capitalize
                app_name=$(echo "$bundle_id" | awk -F. '{print $NF}' | sed 's/^./\U&/')
            fi

            # Try again with extracted name
            icon=$("$CONFIG_DIR/plugins/icon_map_fn.sh" "$app_name")
        fi

        APP_ICONS="$APP_ICONS$icon "
    done < <(echo "$workspace_info" | jq -r '.windows[].bundle_id')

    # Trim trailing space
    APP_ICONS=$(echo "$APP_ICONS" | xargs)

    # If no apps, show empty circle
    if [ -z "$APP_ICONS" ]; then
        APP_ICONS="⏺︎"
    fi

    # Highlight the focused workspace
    if [ "$workspace_name" == "$FOCUSED_WORKSPACE" ]; then
        sketchybar --set $NAME label="$APP_ICONS" background.drawing=on
    else
        sketchybar --set $NAME label="$APP_ICONS" background.drawing=off
    fi
}

update_workspace_icon $1
