#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
plugin=${AYU_NVIM_PATH:-"$HOME/.local/share/nvim/site/pack/dots/opt/neovim-ayu"}
if [[ ! -d "$plugin" ]]; then
  printf '%s\n' 'SKIP Neovim Ayu reload: plugin not installed'
  exit 0
fi
cd "$root"
AYU_NVIM_PATH="$plugin" nvim --headless -u NONE -i NONE \
  --cmd 'lua vim.opt.rtp:append(vim.env.AYU_NVIM_PATH)' \
  '+lua local ok, err = pcall(dofile, "tests/test-nvim-palette.lua"); if not ok then print(err); vim.cmd("cquit 1") end' \
  +qa
