#!/usr/bin/env bash
# Proxy-path observation only: no service restarts or direct fallback.
set -euo pipefail
port=${HEALTH_PORT:-7896}
state_dir=${STATE_DIRECTORY:-/run/vpn-health}
mkdir -p "$state_dir"
previous=$(cat "$state_dir/status" 2>/dev/null || true)
status=failed
# Independent endpoints: one remote outage must not trigger a false alarm.
for url in https://www.gstatic.com/generate_204 https://cp.cloudflare.com/generate_204; do
  code=$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --noproxy '' --proxy "socks5h://127.0.0.1:$port" \
    --connect-timeout 5 --max-time 12 "$url" || true)
  if [[ $code == 204 ]]; then
    status=healthy
    break
  fi
done
printf '%s\n' "$status" > "$state_dir/status"
if [[ $status != "$previous" ]]; then
  printf 'VPN proxy health: %s (previous: %s)\n' "$status" "${previous:-unknown}"
fi
[[ $status == healthy ]]
