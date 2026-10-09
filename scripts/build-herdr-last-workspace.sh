#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
destination="$HOME/.local/bin"
mkdir -p "$destination"
build=$(mktemp "$destination/.dots-last-workspace.XXXXXX")
trap 'rm -f "$build"' EXIT

# No runtime dependencies. Replace atomically so a failed build leaves the
# previous binary usable and an already running action can finish normally.
CGO_ENABLED=0 go -C "$root/tools/herdr-last-workspace" build \
  -trimpath -ldflags='-s -w' -o "$build" .
chmod 755 "$build"
mv -f "$build" "$destination/dots-last-workspace"
echo "Installed Herdr workspace helper: $destination/dots-last-workspace"
