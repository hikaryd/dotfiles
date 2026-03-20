#!/bin/bash

echo "=== Stopping Komorebi ==="

# Stop skhd
pkill -x skhd 2>/dev/null

# Stop komorebi (also kills komorebi-bar)
komorebic stop

echo "=== Komorebi stopped ==="
