return {
  {
    'simonmclean/triptych.nvim',
    event = 'VeryLazy',
    dependencies = {
      'nvim-lua/plenary.nvim',
      'echasnovski/mini.icons',
      'antosha417/nvim-lsp-file-operations',
    },
    opts = {
      extension_mappings = {
        ['<c-f>'] = {
          mode = 'n',
          fn = function(target, _)
            require('telescope.builtin').live_grep {
              search_dirs = { target.path },
            }
          end,
        },
      },
    },
    keys = {
      { '<leader>-', ':Triptych<CR>' },
    },
  },
  {
    'A7Lavinraj/fyler.nvim',
    dependencies = { 'echasnovski/mini.icons' },
    event = 'VeryLazy',
    opts = {
      views = {
        confirm = {
          width = 0.2,
          height = 1,
          kind = 'split:leftmost',
          border = 'single',
          buf_opts = {
            buflisted = false,
            modifiable = false,
          },
          win_opts = {
            winhighlight = 'Normal:Normal,FloatBorder:FloatBorder,FloatTitle:FloatTitle',
            wrap = false,
          },
        },
        explorer = {
          width = 0.2,
          height = 1,
          kind = 'split:leftmost',
          border = 'single',
          buf_opts = {
            buflisted = false,
            buftype = 'acwrite',
            filetype = 'fyler',
            syntax = 'fyler',
          },
          win_opts = {
            concealcursor = 'nvic',
            conceallevel = 3,
            cursorline = true,
            number = true,
            relativenumber = true,
            winhighlight = 'Normal:Normal,FloatBorder:FloatBorder,FloatTitle:FloatTitle',
            wrap = false,
          },
        },
      },
    },
  },
}
