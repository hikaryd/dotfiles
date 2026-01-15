#!/bin/bash

LAUNCHAGENTS_DIR="$HOME/Library/LaunchAgents"
SERVICES_DIR="$HOME/.config/komorebi/services"
SKHD_CONFIG="$HOME/.config/komorebi/config/skhdrc"

echo "=== Installing Komorebi Services ==="

# Copy plist files to LaunchAgents
cp "$SERVICES_DIR/com.komorebi.plist" "$LAUNCHAGENTS_DIR/"
cp "$SERVICES_DIR/com.komorebi.bar.plist" "$LAUNCHAGENTS_DIR/"

# Unload existing services (ignore errors if not loaded)
launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.plist" 2>/dev/null
launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.bar.plist" 2>/dev/null

# Stop skhd service
skhd --stop-service 2>/dev/null

# Kill any remaining processes
pkill -9 komorebi 2>/dev/null
pkill -9 komorebi-bar 2>/dev/null

sleep 1

# Load services
echo "Loading komorebi..."
launchctl load "$LAUNCHAGENTS_DIR/com.komorebi.plist"

sleep 1

echo "Loading komorebi-bar..."
launchctl load "$LAUNCHAGENTS_DIR/com.komorebi.bar.plist"

echo "Starting skhd..."
export SKHD_CONFIG_HOME="$HOME/.config/komorebi/config"
skhd --start-service

echo "=== Services started ==="
echo "Logs: /tmp/komorebi.log, /tmp/komorebi-bar.log"
