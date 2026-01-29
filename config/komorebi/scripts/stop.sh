#!/bin/bash

LAUNCHAGENTS_DIR="$HOME/Library/LaunchAgents"
USER_ID=$(id -u)

echo "=== Stopping Komorebi Services ==="

# Stop skhd first
echo "Stopping skhd..."
skhd --stop-service 2>/dev/null
pkill -x skhd 2>/dev/null

# Unload launch agents (try both old and new methods)
echo "Unloading launch agents..."
launchctl bootout "gui/$USER_ID/com.komorebi.bar" 2>/dev/null
launchctl bootout "gui/$USER_ID/com.komorebi" 2>/dev/null
launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.bar.plist" 2>/dev/null
launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.plist" 2>/dev/null

# Kill processes
echo "Killing processes..."
pkill -9 -x komorebi-bar 2>/dev/null
pkill -9 -x komorebi 2>/dev/null

sleep 1

# Double check and force kill if still running
if pgrep -x komorebi >/dev/null || pgrep -x komorebi-bar >/dev/null; then
    echo "Force killing remaining processes..."
    killall -9 komorebi komorebi-bar 2>/dev/null
    sleep 1
fi

# Remove plist files from LaunchAgents
echo "Removing plist files..."
rm -f "$LAUNCHAGENTS_DIR/com.komorebi.plist"
rm -f "$LAUNCHAGENTS_DIR/com.komorebi.bar.plist"

# Verify
if pgrep -x komorebi >/dev/null || pgrep -x komorebi-bar >/dev/null; then
    echo "WARNING: Some processes may still be running"
    pgrep -l komorebi
else
    echo "=== All services stopped and removed ==="
fi
