#!/usr/bin/env bash
# Build native helpers once; hotkeys execute binaries, never go run/interpreters.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
case ${1:-} in
  '') ;;
  --vpn-linux) ;;
  *) printf 'Usage: build-go-tools.sh [--vpn-linux]\n' >&2; exit 2 ;;
esac

build_dir="$root/build/bin"
mkdir -p "$build_dir"
if [[ ${1:-} == --vpn-linux ]]; then
  arch=${VPN_GOARCH:-amd64}
  case "$arch" in amd64|arm64) ;; *) printf 'Unsupported VPN_GOARCH\n' >&2; exit 2 ;; esac
  temporary=$(mktemp "$build_dir/.dots-vpn-linux.XXXXXX")
  trap 'rm -f "$temporary"' EXIT
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go -C "$root/tools/dots" build \
    -trimpath -ldflags='-s -w' -o "$temporary" ./cmd/dots-tools
  chmod 755 "$temporary"
  mv -f "$temporary" "$build_dir/dots-vpn-linux-$arch"
  printf 'Built %s\n' "$build_dir/dots-vpn-linux-$arch"
  exit 0
fi

storage="${XDG_DATA_HOME:-$HOME/.local/share}/dots/bin"
mkdir -p "$storage"
temporary=$(mktemp "$storage/.dots-tools.XXXXXX")
temporary_kube=$(mktemp "$storage/.dots-kube.XXXXXX")
trap 'rm -f "$temporary" "$temporary_kube"' EXIT
escaped_root=${root//\\/\\\\}
escaped_root=${escaped_root//\"/\\\"}
CGO_ENABLED=0 go -C "$root/tools/dots" build -trimpath \
  -ldflags="-s -w -X \"main.defaultRoot=$escaped_root\"" -o "$temporary" ./cmd/dots-tools
CGO_ENABLED=0 go -C "$root/tools/dots" build -trimpath -ldflags='-s -w' \
  -o "$temporary_kube" ./cmd/dots-kube
chmod 755 "$temporary" "$temporary_kube"
mv -f "$temporary" "$storage/dots-tools"
mv -f "$temporary_kube" "$storage/dots-kube"
ln -sfn "$storage/dots-tools" "$build_dir/dots-tools"
ln -sfn "$storage/dots-kube" "$build_dir/dots-kube"
kube_commands=$("$storage/dots-kube" commands)
for name in dots dots-nu ai_helper dots-vpn dots-task-titles pi-install; do
  ln -sfn dots-tools "$build_dir/$name"
done
for name in $kube_commands; do
  ln -sfn dots-kube "$build_dir/$name"
done
mkdir -p "$HOME/.local/bin"
# Dotbot manages the public desktop aliases; admin helpers must be available
# before Dotbot runs the rest of the installation steps.
for name in dots-tools dots-kube dots-vpn dots-task-titles $kube_commands; do
  target="$HOME/.local/bin/$name"
  source="$build_dir/$name"
  if [[ -e "$target" || -L "$target" ]]; then
    if [[ ! -L "$target" || $(readlink "$target") != "$source" ]]; then
      # Only the known standalone Go watchers from older dots installations may
      # be replaced. Never overwrite an unrelated command or a foreign symlink.
      legacy_module=''
      case "$name" in
        dots-kube) legacy_module=dots/native/cmd/dots-kube ;;
        kube-deploy-watch) legacy_module=dots/kube-rollout ;;
        kube-log-watch) legacy_module=dots/kube-log-watch ;;
        kube-pod-analyze) legacy_module=dots/kube-pod-analyze ;;
      esac
      if [[ -L "$target" || ! -f "$target" || -z "$legacy_module" ]] ||
         ! go version -m "$target" 2>/dev/null | grep -Eq "^[[:space:]]+path[[:space:]]+$legacy_module$"; then
        printf 'Existing command preserved: %s\n' "$target" >&2
        exit 1
      fi
    fi
  fi
  ln -sfn "$source" "$target"
done
# Existing Nushell sessions may still have the old zsh wrappers ahead of
# ~/.local/bin in PATH. Remove only links owned by this checkout's bridge.
bridge_cache="${XDG_CACHE_HOME:-$HOME/.cache}/dots/zsh-bridge"
if [[ -d "$bridge_cache/bin" && ! -L "$bridge_cache" && ! -L "$bridge_cache/bin" ]]; then
  for name in $kube_commands; do
    target="$bridge_cache/bin/$name"
    if [[ -L "$target" && $(readlink "$target") == "$root/scripts/dots-zsh" ]]; then
      rm "$target"
    fi
  done
fi
printf 'Built native helpers: %s\nStandalone Kubernetes: %s\n' "$storage/dots-tools" "$storage/dots-kube"
