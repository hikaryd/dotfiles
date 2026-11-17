#!/usr/bin/env bash
# Собираем один раз: хоткеи запускают бинарник, а не go run или интерпретатор.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
case ${1:-} in
  '') ;;
  --vpn-linux) ;;
  *) printf 'Использование: build-go-tools.sh [--vpn-linux]\n' >&2; exit 2 ;;
esac

build_dir="$root/build/bin"
mkdir -p "$build_dir"
if [[ ${1:-} == --vpn-linux ]]; then
  arch=${VPN_GOARCH:-amd64}
  case "$arch" in amd64|arm64) ;; *) printf 'Неподдерживаемый VPN_GOARCH\n' >&2; exit 2 ;; esac
  temporary=$(mktemp "$build_dir/.dots-vpn-linux.XXXXXX")
  trap 'rm -f "$temporary"' EXIT
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go -C "$root/tools/dots" build \
    -trimpath -ldflags='-s -w' -o "$temporary" ./cmd/dots-tools
  chmod 755 "$temporary"
  mv -f "$temporary" "$build_dir/dots-vpn-linux-$arch"
  printf 'Собрано: %s\n' "$build_dir/dots-vpn-linux-$arch"
  exit 0
fi

storage="${XDG_DATA_HOME:-$HOME/.local/share}/dots/bin"
mkdir -p "$storage"
temporary=$(mktemp "$storage/.dots-tools.XXXXXX")
trap 'rm -f "$temporary"' EXIT
escaped_root=${root//\\/\\\\}
escaped_root=${escaped_root//\"/\\\"}
CGO_ENABLED=0 go -C "$root/tools/dots" build -trimpath \
  -ldflags="-s -w -X \"main.defaultRoot=$escaped_root\"" -o "$temporary" ./cmd/dots-tools
chmod 755 "$temporary"
mv -f "$temporary" "$storage/dots-tools"
ln -sfn "$storage/dots-tools" "$build_dir/dots-tools"
for name in dots dots-nu ai_helper dots-vpn; do
  ln -sfn dots-tools "$build_dir/$name"
done
mkdir -p "$HOME/.local/bin"
# Остальные псевдонимы ставит dotbot, здесь только служебные.
for name in dots-tools dots-vpn; do
  target="$HOME/.local/bin/$name"
  source="$build_dir/$name"
  if [[ -e "$target" || -L "$target" ]]; then
    if [[ ! -L "$target" || $(readlink "$target") != "$source" ]]; then
      printf 'Команда уже существует, не трогаю: %s\n' "$target" >&2
      exit 1
    fi
  fi
  ln -sfn "$source" "$target"
done
printf 'Собрано: %s\n' "$storage/dots-tools"
