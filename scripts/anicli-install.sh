#!/bin/sh
set -eu

for dependency in uv mpv; do
	if ! command -v "$dependency" >/dev/null 2>&1; then
		printf 'anicli: missing %s. Install dependencies with: brew install uv mpv\n' "$dependency" >&2
		exit 1
	fi
done

# Use the verified runtime/provider pair, not the system Python or a global pip.
# No --force: uv must preserve executable collisions owned by other tools.
exec uv tool install --python 3.12 --with 'anicli-api==0.12.0' 'anicli-ru==6.1.3'
