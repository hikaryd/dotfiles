#!/bin/sh
# Pinned pure-Go release. Never restart a daemon or overwrite an unknown command.
set -eu
umask 077
script=$0
while [ -L "$script" ]; do
  dir=$(cd -P "$(dirname "$script")" && pwd)
  link=$(readlink "$script")
  case "$link" in /*) script=$link ;; *) script=$dir/$link ;; esac
done
root=$(cd -P "$(dirname "$script")/.." && pwd)
data=${XDG_DATA_HOME:-$HOME/.local/share}
config=${XDG_CONFIG_HOME:-$HOME/.config}
profile=${DOTS_TUIOS_PROFILE:-$data/dots/tuios/profile}
version=0.11.0
binary="$data/dots/tuios/$version/tuios"
if [ ! -x "$binary" ]; then
  platform=$(uname -s)
  arch=$(uname -m)
  case "$platform/$arch" in
    Darwin/arm64) sum=0f2777e9fed0ce12867e6a10892f05a7390244cb24371bdc54ab612da9a8aa4d ;;
    Darwin/x86_64) sum=6d0970274c586ff34f81bcb488fea72213d0d47e125af7cc8fda384fde661881 ;;
    Linux/aarch64|Linux/arm64) arch=arm64; sum=065e44cf5133fdde81e7afd8708c70471331cd3c79a325a67dc34439980ca1ed ;;
    Linux/x86_64) sum=ef22ec82cda0db703f288da0fd7ccaec2617b4643b2e6cda96338b6b8490b691 ;;
    *) printf 'Unsupported TUIOS platform: %s/%s\n' "$platform" "$arch" >&2; exit 1 ;;
  esac
  mkdir -p "$(dirname "$binary")"
  stage=$(mktemp -d "$(dirname "$binary")/.install.XXXXXX")
  trap 'rm -rf "$stage"' EXIT HUP INT TERM
  archive="tuios_${version}_${platform}_${arch}.tar.gz"
  curl --fail --location --retry 2 "https://github.com/Gaurav-Gosain/tuios/releases/download/v$version/$archive" -o "$stage/$archive"
  (cd "$stage" && printf '%s  %s\n' "$sum" "$archive" | shasum -a 256 -c -)
  tar -xzf "$stage/$archive" -C "$stage" tuios
  chmod 755 "$stage/tuios"
  "$stage/tuios" --version | grep -F "tuios version $version [pure-Go backend]" >/dev/null
  mv "$stage/tuios" "$binary"
fi
"$binary" --version | grep -F "tuios version $version [pure-Go backend]" >/dev/null
backup="$HOME/.config/dots-backups/tuios/$(date +%Y%m%d-%H%M%S)-$$"
link_owned() {
  source=$1 target=$2 group=$3
  if [ -L "$target" ] && [ "$(readlink "$target")" = "$source" ]; then return; fi
  mkdir -p "$(dirname "$target")"
  if [ -e "$target" ] || [ -L "$target" ]; then
    mkdir -p "$backup/$group"
    destination="$backup/$group/$(basename "$target")"
    if [ -e "$destination" ] || [ -L "$destination" ]; then
      printf 'Backup name collision, preserving %s\n' "$target" >&2; exit 1
    fi
    mv "$target" "$destination"
  fi
  ln -s "$source" "$target"
}
mkdir -p "$profile/config" "$profile/run" "$profile/state"
chmod 700 "$profile" "$profile/run" "$profile/state"
for command in tuios dots-terminal dots-tuios-shell; do
  link_owned "$root/scripts/$command" "$HOME/.local/bin/$command" bin
done
link_owned "$root/config/tuios" "$config/tuios" config
link_owned "$config/tuios" "$profile/config/tuios" profile
printf 'TUIOS %s installed. Existing daemons and panes were not restarted.\n' "$version"
