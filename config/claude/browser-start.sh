#!/bin/sh
set -eu
umask 077
export DISPLAY=:99 XDG_RUNTIME_DIR=/tmp/browser-runtime
mkdir -p "$HOME/.vnc" "$HOME/firefox" "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
IFS= read -r password
[ "${#password}" -eq 8 ] || exit 2
x11vnc -storepasswd "$password" "$HOME/.vnc/passwd" >/dev/null 2>&1
unset password
mkdir -m 700 /run/guard
IFS= read -r proxy_auth
printf '%s' "$proxy_auth" > /run/guard/proxy-auth.json
unset proxy_auth
Xvfb :99 -screen 0 1440x900x24 -nolisten tcp >/tmp/xvfb.log 2>&1 &
xvfb=$!
wm= vnc= browser=
cleanup() {
    for pid in "$browser" "$vnc" "$wm" "$xvfb"; do
        [ -z "$pid" ] || kill "$pid" 2>/dev/null || true
    done
}
trap cleanup EXIT HUP INT TERM
attempt=0
while [ ! -S /tmp/.X11-unix/X99 ]; do
    kill -0 "$xvfb"
    attempt=$((attempt + 1))
    [ "$attempt" -lt 50 ] || exit 1
    sleep 0.1
done
openbox >/tmp/openbox.log 2>&1 &
wm=$!
x11vnc -display :99 -rfbport 5900 -rfbauth "$HOME/.vnc/passwd" -forever -shared -noxdamage >/tmp/vnc.log 2>&1 &
vnc=$!
firefox --no-remote --profile "$HOME/firefox" about:blank >/tmp/firefox.log 2>&1 &
browser=$!
attempt=0
while [ ! -f /run/guard/auth-ready ]; do
    kill -0 "$xvfb" "$vnc" "$browser"
    attempt=$((attempt + 1))
    [ "$attempt" -lt 80 ] || exit 1
    sleep 0.1
done
touch /tmp/browser-ready
wait "$browser"
