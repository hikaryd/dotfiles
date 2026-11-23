#!/bin/sh
# Формула pg83/tap/shitty ставит только голый st. Dock, Spotlight и Cmd+Tab
# видят настоящий бандл лишь с копией бинарника, а не со скриптом-обёрткой.
set -eu
src=/opt/homebrew/bin/st
if [ ! -x "$src" ]; then
  echo 'shitty не установлен, пропуск Shitty.app'
  exit 0
fi
apps=${DOTS_APPLICATIONS_DIR:-/Applications}
app="$apps/Shitty.app"
stamp="$app/Contents/Resources/source.sha256"
sum=$(shasum -a 256 "$src" | cut -d' ' -f1)
terminal="$HOME/.local/bin/dots-terminal"
# Номер формата бандла: смена раскладки бандла пересобирает его без обновления st.
layout=2
# Developer ID-подпись brew-бинарника ссылается на отсутствующий Info.plist, и ядро
# убивает его при запуске. Для CLI годится только переподписанная копия из бандла.
link_cli() {
  mkdir -p "$HOME/.local/bin"
  ln -sfn "$app/Contents/MacOS/st" "$HOME/.local/bin/st"
}
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$sum $terminal $layout" ]; then
  link_cli
  echo 'Shitty.app актуален'
  exit 0
fi
version=$(basename "$(dirname "$(dirname "$(realpath "$src")")")")
tmp=$(mktemp -d "$apps/.Shitty.app.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/Contents/MacOS" "$tmp/Contents/Resources"
cp "$src" "$tmp/Contents/MacOS/st"
printf '%s %s %s\n' "$sum" "$terminal" "$layout" >"$tmp/Contents/Resources/source.sha256"
# Иконку st ставит себе только в рантайме; Finder и Launchpad берут её из .icns.
# Это единственный встроенный PNG 1024x1024; без него бандл собирается без иконки.
icon=''
if python3 -I -c '
import struct, sys
data = open(sys.argv[1], "rb").read()
sig = b"\x89PNG\r\n\x1a\n"
i = data.find(sig)
while i >= 0:
    if struct.unpack(">II", data[i + 16:i + 24]) == (1024, 1024):
        end = data.find(b"IEND", i) + 8
        open(sys.argv[2], "wb").write(data[i:end])
        sys.exit(0)
    i = data.find(sig, i + 8)
sys.exit(1)
' "$src" "$tmp/icon.png"; then
  iconset=$tmp/Shitty.iconset
  mkdir "$iconset"
  ok=1
  for size in 16 32 128 256 512; do
    sips -z "$size" "$size" "$tmp/icon.png" --out "$iconset/icon_${size}x${size}.png" >/dev/null &&
      sips -z "$((size * 2))" "$((size * 2))" "$tmp/icon.png" --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null ||
      ok=0
  done
  if [ "$ok" = 1 ] && iconutil -c icns "$iconset" -o "$tmp/Contents/Resources/Shitty.icns"; then
    icon='<key>CFBundleIconFile</key><string>Shitty</string>'
  fi
  rm -rf "$iconset" "$tmp/icon.png"
else
  echo 'Иконка в st не найдена, Shitty.app без иконки' >&2
fi
# st запускает $SHELL через execvp и сам сбрасывает SHELL, если пути нет в /etc/shells.
cat >"$tmp/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>st</string>
  <key>CFBundleIdentifier</key><string>local.dots.shitty</string>
  <key>CFBundleName</key><string>Shitty</string>
  <key>CFBundleDisplayName</key><string>Shitty</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$version</string>
  <key>CFBundleVersion</key><string>$version</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
  <key>NSHighResolutionCapable</key><true/>
  $icon
  <key>LSEnvironment</key>
  <dict>
    <key>SHELL</key><string>$terminal</string>
  </dict>
</dict>
</plist>
EOF
# Подпись автора привязана к его Info.plist, с нашим она недействительна.
codesign --force --sign - "$tmp"
# Права не входят в подпись; mktemp и codesign создают файлы по umask.
chmod -R u=rwX,go=rX "$tmp"
rm -rf "$app"
mv "$tmp" "$app"
trap - EXIT
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app"
link_cli
echo "Shitty.app $version собран в $apps"
