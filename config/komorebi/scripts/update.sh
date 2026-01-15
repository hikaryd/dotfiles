#!/bin/bash
set -e

REPO_URL="git@github.com:KomoCorp/komorebi-for-mac.git"
TMP_DIR="/tmp/komorebi-build"
INSTALL_DIR="$HOME/.local/bin"
LAUNCHAGENTS_DIR="$HOME/Library/LaunchAgents"

echo "=== Komorebi Update Script ==="

# Ensure install directory exists
mkdir -p "$INSTALL_DIR"

# Stop services
echo "Stopping services..."
launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.plist" 2>/dev/null || true
launchctl unload "$LAUNCHAGENTS_DIR/com.komorebi.bar.plist" 2>/dev/null || true

# Kill any remaining processes
pkill -9 komorebi 2>/dev/null || true
pkill -9 komorebi-bar 2>/dev/null || true

sleep 1

# Clean up previous build directory if exists
rm -rf "$TMP_DIR"

# Clone repository
echo "Cloning repository..."
git clone --depth 1 "$REPO_URL" "$TMP_DIR"

cd "$TMP_DIR"

# Build release
echo "Building komorebi (this may take a while)..."
cargo build --release

# Copy binaries
echo "Installing binaries to $INSTALL_DIR..."
cp target/release/komorebi "$INSTALL_DIR/"
cp target/release/komorebi-bar "$INSTALL_DIR/"

# Clean up
echo "Cleaning up..."
cd /
rm -rf "$TMP_DIR"

# Restart services
echo "Restarting services..."
launchctl load "$LAUNCHAGENTS_DIR/com.komorebi.plist"
sleep 1
launchctl load "$LAUNCHAGENTS_DIR/com.komorebi.bar.plist"

echo "=== Update complete! ==="
echo "Binaries installed to: $INSTALL_DIR"
echo "Services restarted."
