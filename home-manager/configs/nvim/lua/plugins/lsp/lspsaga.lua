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
    end,
  },
}
