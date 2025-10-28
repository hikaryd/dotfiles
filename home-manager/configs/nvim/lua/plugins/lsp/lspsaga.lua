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
          -- kind = require('catppuccin.groups.integrations.lsp_saga').custom_kind(),
          code_action = { extend_gitsigns = true, show_server_name = true, border = 'rounded' },
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

        signature = {
          enable = true,
          auto_open = true,
        },

        diagnostic = {
          on_insert = false,
          show_code_action = true,
          border = 'rounded',
        },

        finder = {
          layout = 'vertical',
          border = 'rounded',
          keys = { quit = { 'q', '<ESC>' } },
        },

        definition = {
          keys = { edit = '<CR>', vsplit = 'v', split = 's', quit = 'q' },
        },

        lightbulb = { enable = false, sign = false },
      }
    end,
  },
}
