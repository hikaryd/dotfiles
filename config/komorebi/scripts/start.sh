#!/bin/bash

echo "=== Starting Komorebi ==="

# Start komorebi + komorebi-bar
komorebic start --bar

# Start skhd with komorebi config
skhd --config ~/.config/komorebi/skhdrc &

echo "=== Komorebi started ==="
