vim.g.loaded_ruby_provider = 0
vim.g.loaded_perl_provider = 0
vim.g.loaded_node_provider = 0

vim.opt.shadafile = 'NONE'
vim.schedule(function()
  vim.opt.shadafile = ''
end)

-- Basic options
vim.g.mapleader = ' '
vim.g.maplocalleader = '\\'
vim.cmd 'set modifiable'

-- Enable bytecode cache (нативно в 0.10+)
if vim.loader and vim.loader.enable then
  vim.loader.enable()
end

-- Load core options first
require 'core.options'

pcall(require, 'impatient')

-- Load plugins via native pack manager
require('core.pack').setup(require('plugins'))

-- Finally load the rest of core
require('core').setup()

if vim.fn.getenv 'TERM_PROGRAM' == 'ghostty' then
  vim.opt.title = true
  vim.opt.titlestring = "%{fnamemodify(getcwd(), ':t')}"
end
