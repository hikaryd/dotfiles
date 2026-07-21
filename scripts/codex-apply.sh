#!/usr/bin/env bash
# Merge the versioned, user-owned Codex settings into ~/.codex/config.toml.

set -euo pipefail

CODEX_DIR="${CODEX_CONFIG_DIR:-${CODEX_HOME:-$HOME/.codex}}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMPLATE="$REPO_DIR/config/codex/config.toml"
RULES="$REPO_DIR/config/codex/rules/dots.rules"
TARGET="$CODEX_DIR/config.toml"
SECRETS="$CODEX_DIR/.mcp-secrets"

if [[ ! -f $TEMPLATE ]]; then
  echo "codex-apply: template not found: $TEMPLATE" >&2
  exit 1
fi
if [[ ! -f $RULES ]]; then
  echo "codex-apply: rules file not found: $RULES" >&2
  exit 1
fi

mkdir -p "$CODEX_DIR"

TARGET_PARENT="$(cd "$(dirname "$TARGET")" && pwd -P)"
TARGET_PATH="$TARGET_PARENT/$(basename "$TARGET")"
case "$TARGET_PATH" in
  "$REPO_DIR"|"$REPO_DIR"/*)
    echo "codex-apply: refusing to use a target inside the dotfiles repository: $TARGET_PATH" >&2
    exit 1
    ;;
esac

echo "codex-apply: applying stable config to $TARGET"

install_rules_link() {
  local rules_dir rules_target backup
  rules_dir="$CODEX_DIR/rules"
  rules_target="$rules_dir/dots.rules"
  mkdir -p "$rules_dir"

  if [[ -L $rules_target && $(readlink "$rules_target") == "$RULES" ]]; then
    echo "  rules link: unchanged -> $rules_target"
    return 0
  fi

  backup=""
  if [[ -e $rules_target || -L $rules_target ]]; then
    backup="$rules_target.bak-$(date +%Y%m%d%H%M%S).$$"
    mv "$rules_target" "$backup"
    echo "  existing rules backed up: $(basename "$backup")"
  fi

  if ln -s "$RULES" "$rules_target"; then
    echo "  rules link: installed -> $rules_target"
    return 0
  fi

  if [[ -n $backup && ( -e $backup || -L $backup ) ]]; then
    mv "$backup" "$rules_target"
  fi
  echo "codex-apply: failed to install rules link: $rules_target" >&2
  return 1
}

if [[ -f $SECRETS ]]; then
  chmod 600 "$SECRETS"
  set -a
  # shellcheck disable=SC1090
  . "$SECRETS"
  set +a
  echo "  secrets loaded from $SECRETS"
else
  echo "  warning: $SECRETS not found"
  echo "  secret-backed sections will be preserved if already configured, otherwise skipped"
  echo "  cp '$REPO_DIR/config/codex/mcp-secrets.example.sh' '$SECRETS'"
fi

APPLY_ARGS=(
  apply
  --template "$TEMPLATE"
  --target "$TARGET"
)
if CODEX_BIN="$(command -v codex 2>/dev/null)"; then
  RULES_VALIDATION_HOME="$(mktemp -d "${TMPDIR:-/tmp}/codex-rules-validate.XXXXXX")"
  if ! CODEX_HOME="$RULES_VALIDATION_HOME" "$CODEX_BIN" execpolicy check \
      --rules "$RULES" -- npm --version >/dev/null; then
    rm -rf "$RULES_VALIDATION_HOME"
    echo "codex-apply: rules validation failed: $RULES" >&2
    exit 1
  fi
  rm -rf "$RULES_VALIDATION_HOME"
  echo "  Codex rules validation: passed"
  APPLY_ARGS+=(--codex-bin "$CODEX_BIN")
else
  echo "  warning: codex command not found; semantic config/rules validation skipped"
  echo "  warning: portable rules link was not installed"
fi

if ! python3 "$REPO_DIR/scripts/codex_config.py" "${APPLY_ARGS[@]}"; then
  echo "codex-apply: failed; live config was not replaced" >&2
  exit 1
fi

if [[ -n ${CODEX_BIN:-} ]]; then
  install_rules_link
fi

echo "codex-apply: done"
