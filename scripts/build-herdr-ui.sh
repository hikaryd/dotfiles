#!/usr/bin/env bash
set -euo pipefail
umask 077

# --ensure is the launcher's cached, retry-throttled mode. A manual invocation
# always retries. Build only the installed release; never replace/stop a server.
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ensure=0
if [[ ${1:-} == --ensure ]]; then ensure=1; shift; fi
[[ $# -le 1 ]] || { echo 'Usage: build-herdr-ui.sh [--ensure] [version]' >&2; exit 2; }
if [[ $# -eq 1 ]]; then
  version=$1
else
  base_version=$("$root/scripts/herdr" --version)
  version=${base_version#herdr }
fi
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Herdr UI patch requires a stable release version' >&2; exit 2; }
expected_commit=
case "$version" in
  0.9.1) expected_commit=065ef9d6a531c49fb8bee7e818ef837065b21ee9 ;;
  0.9.4) expected_commit=7e26bad94044eba205033111a0ce8033ede7e24f ;;
esac
# One maintained patch; compatibility is checked against each exact release.
patch="$root/patches/herdr/centered-rounded-tabs.patch"
patch_hash=$(shasum -a 256 "$patch")
patch_hash=${patch_hash%% *}
data="${XDG_DATA_HOME:-$HOME/.local/share}/dots/herdr"
cache="${XDG_CACHE_HOME:-$HOME/.cache}/dots/herdr"
destination="$data/$version"
lock="$data/build.lock"
failure="$destination/.build-failed"
cache_valid() {
  [[ -x $destination/herdr && -f $destination/patch-sha256 ]] &&
    [[ $(< "$destination/patch-sha256") == "$patch_hash" ]] &&
    [[ $("$destination/herdr" --version) == "herdr $version" ]]
}
recent_failure() {
  [[ $ensure == 1 && -f $failure ]] || return 1
  local failed_hash failed_at now
  read -r failed_hash failed_at < "$failure" || return 1
  [[ $failed_hash == "$patch_hash" && $failed_at =~ ^[0-9]{1,10}$ ]] || return 1
  now=$(date +%s)
  [[ $failed_at -le $now && $((now - failed_at)) -lt 900 ]]
}
if [[ $ensure == 1 ]] && cache_valid; then exit 0; fi
if recent_failure; then echo 'Previous build failed; automatic retry is deferred for 15 minutes.' >&2; exit 1; fi
mkdir -p "$destination" "$cache"
command -v shlock >/dev/null || { echo 'Herdr UI builds need macOS shlock' >&2; exit 1; }
# shlock is atomic and reclaims locks whose owner has exited. Serialize all
# versions so concurrent clients also share one toolchain download/build cache.
waited=0
until shlock -p $$ -f "$lock"; do
  [[ $waited -lt 900 ]] || { echo 'Timed out waiting for the Herdr UI build lock' >&2; exit 1; }
  sleep 1
  waited=$((waited + 1))
done
build_dir=
attempt=0
cleanup() {
  local status=$?
  if [[ $attempt == 1 && $status != 0 && $status != 130 && $status != 143 ]]; then
    printf '%s %s\n' "$patch_hash" "$(date +%s)" > "$failure"
  fi
  [[ -z $build_dir ]] || rm -rf "$build_dir"
  rm -f "$destination/herdr.new" "$lock"
  return "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# A concurrent launcher may have completed this exact build while we waited.
if [[ $ensure == 1 ]] && cache_valid; then exit 0; fi
if recent_failure; then echo 'Previous build failed; automatic retry is deferred for 15 minutes.' >&2; exit 1; fi
attempt=1
if [[ $ensure == 1 ]] && { : >&3; } 2>/dev/null; then
  printf 'Herdr: building rounded, centered tabs for %s (first build may take several minutes)…\n' "$version" >&3
else
  printf 'Building rounded, centered Herdr tabs for %s…\n' "$version" >&2
fi
command -v git >/dev/null
command -v cargo >/dev/null || { echo 'Herdr UI builds need Cargo/Rust (brew install rust)' >&2; exit 1; }
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/dots-herdr-ui.XXXXXX")
git clone -q --depth 1 --branch "v$version" --filter=blob:none \
  https://github.com/herdrdev/herdr.git "$build_dir/source"
cd "$build_dir/source"
commit=$(git rev-parse HEAD)
[[ $commit =~ ^[0-9a-f]{40}$ ]] || { echo 'Invalid upstream commit' >&2; exit 1; }
if [[ -n $expected_commit && $commit != "$expected_commit" ]]; then
  echo 'Herdr tag changed; refusing to apply the local UI patch' >&2
  exit 1
fi
git apply --check "$patch"
git apply "$patch"

# Homebrew can upgrade Zig independently. Keep a verified 0.16.0 compiler in
# our private cache instead of changing the user's globally installed version.
zig=${ZIG:-}
if [[ -z $zig ]] && command -v zig >/dev/null && [[ $(zig version) == 0.16.0 ]]; then
  zig=$(command -v zig)
fi
if [[ -z $zig ]]; then
  case "$(uname -s)/$(uname -m)" in
    Darwin/arm64)
      platform=aarch64-macos
      checksum=b23d70deaa879b5c2d486ed3316f7eaa53e84acf6fc9cc747de152450d401489 ;;
    Darwin/x86_64)
      platform=x86_64-macos
      checksum=0387557ed1877bc6a2e1802c8391953baddba76081876301c522f52977b52ba7 ;;
    *) echo 'Automatic Zig provisioning supports macOS only; set ZIG to 0.16.0.' >&2; exit 1 ;;
  esac
  toolchain="$cache/zig-$platform-0.16.0"
  if [[ ! -x $toolchain/zig ]]; then
    curl --proto '=https' --tlsv1.2 -fsSL --retry 2 --connect-timeout 10 --max-time 300 \
      "https://ziglang.org/download/0.16.0/zig-$platform-0.16.0.tar.xz" -o "$build_dir/zig.tar.xz"
    printf '%s  %s\n' "$checksum" "$build_dir/zig.tar.xz" | shasum -a 256 -c -
    tar -xf "$build_dir/zig.tar.xz" -C "$build_dir"
    # Move only a complete, verified toolchain into the shared cache.
    rm -rf "$toolchain"
    mv "$build_dir/zig-$platform-0.16.0" "$toolchain"
  fi
  zig="$toolchain/zig"
fi
[[ $("$zig" version) == 0.16.0 ]] || { echo 'Herdr UI build needs Zig 0.16.0; check ZIG.' >&2; exit 1; }
export ZIG="$zig"
mkdir -p "${CARGO_TARGET_DIR:-$cache/target}"
CARGO_TARGET_DIR=$(cd "${CARGO_TARGET_DIR:-$cache/target}" && pwd)
export CARGO_TARGET_DIR
cargo test --locked --bin herdr tab_group_is_centered_and_uses_rounded_caps | tee "$build_dir/test.log"
grep -Eq 'test result: ok\. [1-9][0-9]* passed;' "$build_dir/test.log" || { echo 'No tab-rendering regression tests ran' >&2; exit 1; }
cargo build --locked --release --bin herdr
[[ $("$CARGO_TARGET_DIR/release/herdr" --version) == "herdr $version" ]] || { echo 'Built client version does not match the installed release' >&2; exit 1; }
install -m 755 "$CARGO_TARGET_DIR/release/herdr" "$destination/herdr.new"
mv "$destination/herdr.new" "$destination/herdr"
printf '%s\n' "$commit" > "$destination/source-commit"
printf '%s\n' "$patch_hash" > "$destination/patch-sha256.new"
mv "$destination/patch-sha256.new" "$destination/patch-sha256"
rm -f "$failure"
echo "Installed rounded, centered Herdr client: $destination/herdr"
