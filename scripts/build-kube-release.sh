#!/usr/bin/env bash
# Shareable Kubernetes tool: no checkout path, credentials, shell or Go runtime.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
case ${1:-} in
  ''|--help|-h) ;;
  *) printf 'Usage: build-kube-release.sh [--help]\n' >&2; exit 2 ;;
esac
if [[ ${1:-} == --help || ${1:-} == -h ]]; then
  printf 'Build macOS/Linux arm64/amd64 archives in build/releases/kube.\nOverride KUBE_VERSION or KUBE_RELEASE_DIR if needed.\n'
  exit 0
fi
version=${KUBE_VERSION:-0.1.0}
[[ $version =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]*$ ]] || { printf 'Invalid KUBE_VERSION\n' >&2; exit 2; }
output=${KUBE_RELEASE_DIR:-$root/build/releases/kube}
mkdir -p "$output"
stage=$(mktemp -d "$output/.stage.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT
for platform in darwin-arm64 darwin-amd64 linux-arm64 linux-amd64; do
  os=${platform%-*}; arch=${platform#*-}
  bundle="$stage/dots-kube-$version-$platform"
  mkdir -p "$bundle"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go -C "$root/tools/dots" build -trimpath \
    -ldflags="-s -w -X dots/native/internal/kube.Version=$version" \
    -o "$bundle/dots-kube" ./cmd/dots-kube
  chmod 755 "$bundle/dots-kube"
  cat > "$bundle/README.txt" <<'INSTRUCTIONS'
DOTS KUBE
Standalone Kubernetes CLI and Ayu terminal UI. macOS or Linux; no Go,
repository, zsh or gum required. Install kubectl separately, plus any
authentication plugins required by your administrator's kubeconfig.

1. Obtain a kubeconfig safely from your administrator. Treat it as a secret.
2. Run: ./dots-kube setup
3. Mark one or more files (Space/Ctrl+A); Ctrl+O browses other folders.
4. Bind each environment to FILE -> CONTEXT -> NAMESPACE; review and save.
   Setup remembers your choice. No shell KUBECONFIG changes or cluster requests.
5. Add $HOME/.local/bin to PATH in your shell configuration if needed.
6. Run: dots-kube config; dots-kube doctor; kpstage get pods

Explicit setup:
  ./dots-kube setup stage --import /path/to/config.yaml \
    --context original --namespace application --yes

Each environment can use a separate file, even when cluster/user/context
names are identical across files. Credentials are never merged across environments.
Source paths are remembered in ~/.config/dots-kube/sources.yaml; native contexts
live in the adjacent contexts/ directory, without any credentials. Supplied
kubeconfigs (including read-only files) are not modified. Environment aliases pin
context/namespace per call; ordinary kubectl and global current-context are unchanged.
Before setup, standard KUBECONFIG / ~/.kube/config loading is used.
Esc goes back without saving. Explicit import is available with --import FILE.
kpstage config shows the stage connection; kpstage setup configures only stage.
kpstage namespace reads the namespace list and saves your choice after confirmation.
In setup, Ctrl+O on the namespace page fetches the list using a read-only request.
Offline/manual entry remains available. Listing without writes: --list --json.

logs-multi without arguments: search the pod list, Space/Tab mark, Enter follow.
Full records are the default; m toggles optional JSON summaries. Long lines wrap.
logs / logs-multi: Space pause, / search, f filter, arrows select,
Enter full record, PgUp/PgDn scroll details, Esc/q/Ctrl+C exit.
Pipes and saved logs remain undecorated. NO_COLOR disables colors.
kpstage-rollout-restart selects Deployment/StatefulSet/DaemonSet workloads and
requests their rollouts without waiting for each pod. Kubernetes owns the strategy.
Paused/OnDelete targets are rejected; use pod-restart for deliberate selected pods.
Prod mutations need interactive yes or deliberate KPROD_CONFIRMED=1.

Run ./dots-kube --help for all commands; preview needs no cluster or kubectl.
INSTRUCTIONS
  archive="dots-kube-$version-$platform.tar.gz"
  tar -czf "$stage/$archive" -C "$bundle" dots-kube README.txt
  mv -f "$stage/$archive" "$output/$archive"
  printf 'Built %s\n' "$output/$archive"
done
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$output" && sha256sum "dots-kube-$version-"*.tar.gz > "$stage/SHA256SUMS")
else
  (cd "$output" && shasum -a 256 "dots-kube-$version-"*.tar.gz > "$stage/SHA256SUMS")
fi
mv -f "$stage/SHA256SUMS" "$output/SHA256SUMS"
