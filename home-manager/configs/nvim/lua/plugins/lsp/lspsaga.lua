return {
  {
    'nvimdev/lspsaga.nvim',
    event = 'LspAttach',
    dependencies = {
      'nvim-treesitter/nvim-treesitter',
      'nvim-tree/nvim-web-devicons',
    },
    config = function()
      require('lspsaga').setup {
        ui = {
          border = 'rounded',
          kind = require('catppuccin.groups.integrations.lsp_saga').custom_kind(),
          code_action = {
            extend_gitsigns = true,
            show_server_name = true,
            border = 'rounded',
          },
          symbol_in_winbar = {
            enable = true,
            separator = ' › ',
            ignore_patterns = {},
            padding = ' ',
          },
          hover = {
            border = 'rounded',
          },
          signature_help = {
            enable = true,
            trigger_on_typed = true,
            border = 'rounded',
          },
          diagnostic = {
            on_insert = false,
            border = 'rounded',
            show_code_action = true,
          },
        },
        finder = {
          layout = 'vertical',
          border = 'rounded',
          keys = {
            quit = { 'q', '<ESC>' },
          },
        },
        definition = {
          keys = {
            edit = '<CR>',
            vsplit = 'v',
            split = 's',
            quit = 'q',
          },
        },
        lightbulb = {
          enable = false,
          sign = false,
        },
      }

      local keymap = vim.keymap.set
      local opts = { noremap = true, silent = true }

      keymap('n', 'gd', '<cmd>Lspsaga goto_definition<CR>', opts)
      keymap('n', 'gr', '<cmd>Lspsaga finder<CR>', opts)
      keymap('n', 'K', '<cmd>Lspsaga hover_doc<CR>', opts)
      keymap('n', '<leader>ca', '<cmd>Lspsaga code_action<CR>', opts)
      keymap('v', '<leader>ca', '<cmd><C-U>Lspsaga range_code_action<CR>', opts)
      keymap('n', 'rn', '<cmd>Lspsaga rename<CR>', opts)
      keymap('n', 'gj', '<cmd>Lspsaga diagnostic_jump_next<CR>', opts)
      keymap('n', 'gk', '<cmd>Lspsaga diagnostic_jump_prev<CR>', opts)

      keymap('n', '<leader>o', '<cmd>Lspsaga outline<CR>', opts)

      keymap('i', '<C-k>', '<cmd>Lspsaga signature_help<CR>', { silent = true })
      keymap('n', '<leader>sh', '<cmd>Lspsaga signature_help<CR>', opts)
    end,
  },
}
