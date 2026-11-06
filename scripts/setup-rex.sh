#!/usr/bin/env bash
set -euo pipefail

if [[ ! -d /Applications/Rex\ Beta.app && ! -d /Applications/Rex.app ]]; then
  printf '%s\n' 'Rex is not installed. Skipping macOS preferences.'
  exit 0
fi

if [[ ! -x "$HOME/.local/bin/dots-tools" ]]; then
  printf '%s\n' 'Rex preferences unchanged: install the dots-tools launcher first (./install -c steps/tools.yml).' >&2
  exit 1
fi

domain=com.superlogical.rex
defaults write "$domain" Workspace.tabStyle -string horizontal
defaults write "$domain" Workspace.appearance -string dark
defaults write "$domain" Workspace.interfaceStyle -string blendTheme
defaults write "$domain" Workspace.usesPaneGlass -bool true
defaults write "$domain" Workspace.showsPaneHeaders -bool false
defaults write "$domain" Workspace.terminalImproveTextContrast -bool true
defaults write "$domain" Workspace.terminalStartupCommand -string "\"$HOME/.local/bin/dots-tools\" desktop nu"
defaults write "$domain" Workspace.terminalFontName -string SFMono-Regular
defaults write "$domain" Workspace.terminalFontPointSize -float 15
defaults write "$domain" Workspace.terminalFallbackFontNames -array 'Symbols Nerd Font Mono'
defaults write "$domain" Workspace.terminalAllowsLigatures -bool false
defaults write "$domain" Workspace.terminalCellHeightPercent -float 12
defaults write "$domain" Workspace.terminalPaddingX -float 18
defaults write "$domain" Workspace.terminalPaddingY -float 14
defaults write "$domain" Workspace.terminalUsesDisplayP3 -bool true
defaults write "$domain" Workspace.terminalCursorStyle -string bar
defaults write "$domain" Workspace.terminalCursorStyleForced -bool true
defaults write "$domain" Workspace.terminalCursorBlinkIntervalMilliseconds -int 0
defaults write "$domain" Workspace.showsKeySequenceHints -bool true
defaults write "$domain" Workspace.showsProgramStatusInTabs -bool true
defaults write "$domain" Workspace.showsProcessesInActivity -bool true
defaults write "$domain" Workspace.showsProgramActivityNotifications -bool true
defaults write "$domain" Workspace.showsProgramActivityDockBadge -bool true
defaults write "$domain" Workspace.playsProgramActivitySounds -bool false
printf '%s\n' 'Rex preferences saved. Reopen the app to apply them.'

rex="$HOME/.local/bin/rex"
theme="$HOME/.config/rex/ayu-dark"
client=""
if [[ -x "$rex" && -f "$theme" ]] && command -v jq >/dev/null; then
  client=$("$rex" client ls --json | jq -r '
    [.clients[] | select(.info.kind == "app" and .info.platform == "macos" and .connection_state == "connected")]
    | if length == 1 then .[0].client_id else empty end') || client=""
fi
if [[ -n "$client" ]] && "$rex" do -C "$client" client.theme.import "path=$theme" &&
  "$rex" do -C "$client" client.appearance.switch_mode mode=dark &&
  "$rex" do -C "$client" client.theme.change name=ayu-dark; then
  printf '%s\n' 'Rex Ayu theme imported and selected.'
else
  printf '%s\n' 'Ayu setup pending: open Rex, enable local Remote Control in Settings > Server, then rerun bash scripts/setup-rex.sh.'
fi
