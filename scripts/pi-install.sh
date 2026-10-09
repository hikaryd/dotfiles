#!/bin/sh
# Reuse an existing Pi installation; bootstrap the CLI on a new machine.
set -eu

if command -v pi >/dev/null 2>&1; then
  printf 'Pi is already installed: '
  pi --version
  exit 0
fi

bin_dir="$HOME/.local/bin"
if [ -e "$bin_dir/pi" ] || [ -L "$bin_dir/pi" ]; then
  echo "pi-install: existing $bin_dir/pi is not executable or not on PATH; preserved" >&2
  exit 1
fi

# The official installer can leave its launcher outside the current PATH.
managed_pi="$HOME/.pi/agent/bin/pi"
if [ -x "$managed_pi" ]; then
  "$managed_pi" --version
  mkdir -p "$bin_dir"
  ln -s "$managed_pi" "$bin_dir/pi"
  exit 0
fi

if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
  echo 'pi-install: Node.js 22.19+ and npm are required (brew install node)' >&2
  exit 1
fi
if ! node -e 'const [major, minor] = process.versions.node.split(".").map(Number); process.exit(major > 22 || (major === 22 && minor >= 19) ? 0 : 1)'; then
  echo 'pi-install: Pi requires Node.js 22.19 or newer' >&2
  exit 1
fi

# User-owned prefix is already on PATH in the dots shells. No lifecycle scripts.
npm install -g --prefix "$HOME/.local" --ignore-scripts --no-fund --no-audit \
  @earendil-works/pi-coding-agent@1.1.0
"$bin_dir/pi" --version
