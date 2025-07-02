return {
  'bassamsdata/namu.nvim',
  event = 'VeryLazy',
  config = function()
    require('namu').setup {
      namu_symbols = {
        enable = true,
        options = {},
      },
      ui_select = { enable = true },
      colorscheme = {
        enable = false,
      },
    }
    local namu = require 'namu.namu_symbols'
    local colorscheme = require 'namu.colorscheme'
    vim.keymap.set('n', '<leader>ss', namu.show, {
      desc = 'Jump to LSP symbol',
      silent = true,
    })
  end,
}
