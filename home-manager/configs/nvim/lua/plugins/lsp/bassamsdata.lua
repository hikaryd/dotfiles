return {
  'bassamsdata/namu.nvim',
  event = 'VeryLazy',
  priority = 1000,
  config = function()
    require('namu').setup {
      namu_symbols = {
        enable = true,
        options = {},
      },
      ui_select = { enable = true },
      colorscheme = {
        enable = true,
        options = {
          persist = true,
          write_shada = false,
        },
      },
    }
    local namu = require 'namu.namu_symbols'
    local colorscheme = require 'namu.colorscheme'
    vim.keymap.set('n', '<leader>ss', namu.show, {
      desc = 'Jump to LSP symbol',
      silent = true,
    })
    vim.keymap.set('n', '<leader>th', colorscheme.show, {
      desc = 'Colorscheme Picker',
      silent = true,
    })
  end,
}
