#!/usr/bin/env bash
# Package installation stays separate from Dotbot so its failures can be tested.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
if ! command -v brew >/dev/null 2>&1; then
  echo 'Homebrew is not installed. Skipping Brewfile.'
  exit 0
fi
if [[ $(uname -s) == Darwin ]] && {
  ! xcode-select -p >/dev/null 2>&1 || ! xcrun --find clang >/dev/null 2>&1;
}; then
  printf 'Command Line Tools are missing or inactive. Run xcode-select --install, finish the installation, then rerun ./install.\n' >&2
  exit 1
fi

# Recover stale cask receipts without --zap: preserve preferences and user data.
# The override also allows isolated tests and nonstandard application directories.
apps=${DOTS_APPLICATIONS_DIR:-/Applications}
for entry in 'mos:Mos.app' 'duongductrong/snapzy/snapzy:Snapzy.app'; do
  cask=${entry%:*}
  app="$apps/${entry#*:}"
  token=${cask##*/}
  grep -Eq "^cask \"($cask|$token)\"" Brewfile || continue
  if brew list --cask "$token" >/dev/null 2>&1; then
    if [[ ! -d $app ]]; then
      printf 'Homebrew records %s but %s is missing; clearing stale state...\n' "$token" "$app"
      brew uninstall --cask "$cask" --force
    fi
  elif [[ $token == mos && -d $app ]]; then
    version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$app/Contents/Info.plist" 2>/dev/null || true)
    backup="$app.backup-${version:-unknown}-$(date +%Y%m%d%H%M%S)"
    printf 'Found unmanaged Mos.app; moving it to %s before brew bundle...\n' "$backup"
    mv "$app" "$backup"
  fi
done

# Tap URLs remain controlled by Brewfile. Trust only the explicitly selected
# third-party packages, not all present/future contents of each tap.
while IFS=$'\t' read -r tap url; do
  [[ -n $tap ]] || continue
  if [[ -n $url ]]; then brew tap "$tap" "$url"; else brew tap "$tap"; fi
done < <(awk -F '"' '/^tap "/ { print $2 "\t" $4 }' Brewfile)
while read -r kind package; do
  [[ -n $package ]] || continue
  brew tap "${package%/*}"
  case $kind in
    brew) brew trust --formula "$package" ;;
    cask) brew trust --cask "$package" ;;
  esac
done < <(awk -F '"' '/^(brew|cask) "[^\/]+\/[^\/]+\/[^\"]+"/ { kind=$1; gsub(/ /,"",kind); print kind " " $2 }' Brewfile)

echo 'Installing packages from Brewfile ...'
if brew bundle --file Brewfile; then
  echo 'Homebrew bundle installed successfully.'
else
  status=$?
  printf 'Homebrew bundle failed (exit %s). See the package error above.\n' "$status" >&2
  exit "$status"
fi
