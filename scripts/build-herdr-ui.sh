#!/usr/bin/env bash
set -euo pipefail

# Herdr 0.9.1 has no tab alignment/shape setting. Build a small client-only
# rendering patch while leaving the Homebrew installation available.
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version=0.9.1
commit=065ef9d6a531c49fb8bee7e818ef837065b21ee9
patch="$root/patches/herdr/$version-centered-rounded-tabs.patch"
destination="${XDG_DATA_HOME:-$HOME/.local/share}/dots/herdr/$version"

if [[ $(zig version) != 0.16.0 ]]; then
  echo 'Herdr UI build needs Zig 0.16.0' >&2
  exit 1
fi

build_dir=$(mktemp -d "${TMPDIR:-/tmp}/dots-herdr-ui.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT
git clone -q --depth 1 --branch "v$version" --filter=blob:none \
  https://github.com/herdrdev/herdr.git "$build_dir/source"
cd "$build_dir/source"
if [[ $(git rev-parse HEAD) != "$commit" ]]; then
  echo 'Herdr tag changed; refusing to apply the local UI patch' >&2
  exit 1
fi
git apply --check "$patch"
git apply "$patch"
cargo test --locked --bin herdr tab_group_is_centered_and_uses_rounded_caps
cargo build --locked --release --bin herdr
mkdir -p "$destination"
install -m 755 target/release/herdr "$destination/herdr.new"
mv "$destination/herdr.new" "$destination/herdr"
echo "Installed rounded, centered Herdr client: $destination/herdr"
