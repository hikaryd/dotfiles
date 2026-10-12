#!/usr/bin/env bash
# Source after setting NATIVE to the built dots-tools binary.
# Theme, keyboard, terminal ownership and secret masking live exclusively in Go.
ui_message() { local level=$1; shift; printf '%s\n' "$*" | "$NATIVE" ui message --level "$level"; }
step() { printf '%s\n' "$*" | "$NATIVE" ui snapshot --brand "${UI_BRAND:-dots}" --title "$*" --mode "${UI_MODE:-LOCAL CONFIG}"; }
ok() { ui_message success "✓ $*"; }
warn() { ui_message warning "! $*" >&2; }
die() { ui_message error "✗ $*" >&2; exit 1; }
dry() { ui_message info "[dry] $*"; }
ask() { "$NATIVE" ui input --brand "${UI_BRAND:-dots}" --title "$1" --value "${2:-}" --mode "${UI_MODE:-LOCAL CONFIG}"; }
ask_secret() { "$NATIVE" ui input --brand "${UI_BRAND:-dots}" --title "$1" --secret --mode "${UI_MODE:-LOCAL CONFIG}"; }
