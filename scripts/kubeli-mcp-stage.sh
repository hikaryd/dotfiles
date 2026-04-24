#!/usr/bin/env bash
# Wrapper: launches Kubeli MCP with stage-only KUBECONFIG.
# Prevents the MCP server from ever seeing prod kubeconfigs.

set -euo pipefail

DEFAULT_DIR="$HOME/.kube/configs/default"

if [ ! -d "$DEFAULT_DIR" ]; then
  echo "kubeli-mcp-stage: $DEFAULT_DIR does not exist" >&2
  exit 1
fi

KUBECONFIG=$(find "$DEFAULT_DIR" -maxdepth 1 -type f | tr '\n' ':' | sed 's/:$//')

if [ -z "$KUBECONFIG" ]; then
  echo "kubeli-mcp-stage: no kubeconfig files in $DEFAULT_DIR" >&2
  exit 1
fi

export KUBECONFIG
exec /Applications/Kubeli.app/Contents/MacOS/Kubeli --mcp
