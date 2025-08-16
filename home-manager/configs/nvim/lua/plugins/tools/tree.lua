return {
  {
    'nvim-neo-tree/neo-tree.nvim',
    branch = 'v3.x',
    dependencies = {
      'nvim-lua/plenary.nvim',
      'MunifTanjim/nui.nvim',
      'echasnovski/mini.icons',
    },
    lazy = false,
  },
  {
    'A7Lavinraj/fyler.nvim',
    dependencies = { 'echasnovski/mini.icons' },
    event = 'VeryLazy',
    opts = {
      close_on_select = false,
      -- views = {
      --   confirm = {
      --     width = 0.2,
      --     height = 1,
      --     kind = 'split:leftmost',
      --     border = 'single',
      --     buf_opts = {
      --       buflisted = false,
      --       modifiable = false,
      --     },
      --     win_opts = {
      --       winhighlight = 'Normal:Normal,FloatBorder:FloatBorder,FloatTitle:FloatTitle',
      --       wrap = false,
      --     },
      --   },
      --   explorer = {
      --     width = 0.2,
      --     height = 1,
      --     kind = 'split:leftmost',
      --     border = 'single',
      --     buf_opts = {
      --       buflisted = false,
      --       buftype = 'acwrite',
      --       filetype = 'fyler',
      --       syntax = 'fyler',
      --     },
      --     win_opts = {
      --       concealcursor = 'nvic',
      --       conceallevel = 3,
      --       cursorline = true,
      --       number = true,
      --       relativenumber = true,
      --       winhighlight = 'Normal:Normal,FloatBorder:FloatBorder,FloatTitle:FloatTitle',
      --       wrap = false,
      --     },
      --   },
      -- },
    },
  },
}
