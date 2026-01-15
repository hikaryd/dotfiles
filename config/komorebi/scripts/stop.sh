#!/bin/bash

LAUNCHAGENTS_DIR="$HOME/Library/LaunchAgents"

echo "=== Stopping Komorebi Services ==="

launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.bar.plist" 2>/dev/null
launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.plist" 2>/dev/null

skhd --stop-service 2>/dev/null

pkill -9 komorebi 2>/dev/null
pkill -9 komorebi-bar 2>/dev/null

echo "=== Services stopped ==="
