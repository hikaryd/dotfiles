#!/bin/bash
set -euo pipefail
# Never replace an app while its GUI or owned listener might be serving a session.
if launchctl print "gui/$(id -u)/local.dots.antigravity-manager" >/dev/null 2>&1 ||
   pgrep -f '[A]ntigravity Tools.app/Contents/MacOS/antigravity-tools' >/dev/null; then
  echo 'antigravity-tools: update deferred while Manager is running'
  exit 0
fi
command -v brew >/dev/null || { echo 'antigravity-tools: brew not found' >&2; exit 1; }
if ! brew list --cask lbjlaq/antigravity-manager/antigravity-tools >/dev/null 2>&1; then
  echo 'antigravity-tools: cask not installed, skipping update'
  exit 0
fi
export HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_ENV_HINTS=1 NONINTERACTIVE=1
brew upgrade --cask lbjlaq/antigravity-manager/antigravity-tools </dev/null
