-- Bootstrap lazy.nvim
local lazypath = vim.fn.stdpath 'data' .. '/lazy/lazy.nvim'
if not vim.loop.fs_stat(lazypath) then
  vim.fn.system {
    'git',
    'clone',
    '--filter=blob:none',
    'https://github.com/folke/lazy.nvim.git',
    '--branch=stable', -- latest stable release
    lazypath,
  }
end
vim.opt.rtp:prepend(lazypath)

-- Basic options
vim.g.mapleader = ' '
vim.cmd 'set modifiable'

-- Load core options first
require 'core.options'

-- Then initialize lazy.nvim
require('lazy').setup('plugins', {
  root = vim.fn.stdpath 'data' .. '/lazy',
  lockfile = vim.fn.stdpath 'config' .. '/lazy-lock.json',
  rocks = { enabled = false },
  dev = {
    path = '~/.local/share/nvim/nix',
    fallback = false,
  },
  defaults = {
    lazy = true,
    version = false,
  },
  checker = {
    enabled = true,
    notify = false,
  },
  performance = {
    rtp = {
      disabled_plugins = {
        'gzip',
        'tarPlugin',
        'tohtml',
        'tutor',
        'zipPlugin',
      },
    },
  },
})

-- Finally load the rest of core
require('core').setup()
require 'plugins'

if vim.fn.getenv 'TERM_PROGRAM' == 'ghostty' then
  vim.opt.title = true
  vim.opt.titlestring = "%{fnamemodify(getcwd(), ':t')}"
end
