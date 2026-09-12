#!/bin/sh
# Local compilation only. Does not download tools, contact Android, or start a VM.
set -eu
: "${JAVA_HOME:?Set JAVA_HOME to an existing JDK 17 installation}"
SDK=${ANDROID_HOME:-"$HOME/Library/Android/sdk"}
OUT=${1:-/tmp/pixel-avf-probe-build}
HERE=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$OUT/classes" "$OUT/dex"
"$JAVA_HOME/bin/javac" -Xlint:all -Werror -source 8 -target 8 -Xlint:-options \
  -cp "$SDK/platforms/android-35/android.jar" -d "$OUT/classes" "$HERE"/*.java
"$SDK/build-tools/34.0.0/d8" --lib "$SDK/platforms/android-35/android.jar" \
  --output "$OUT/dex" "$OUT/classes"/*.class
printf 'Built %s/dex/classes.dex (not deployed)\n' "$OUT"
