return {
  {
    'marko-cerovac/material.nvim',
    priority = 1000,
    lazy = false,
    config = function()
      require('material').setup {
        contrast = {
          terminal = false, -- Enable contrast for the built-in terminal
          sidebars = false, -- Enable contrast for sidebar-like windows ( for example Nvim-Tree )
          floating_windows = false, -- Enable contrast for floating windows
          cursor_line = true, -- Enable darker background for the cursor line
          lsp_virtual_text = true, -- Enable contrasted background for lsp virtual text
          non_current_windows = true, -- Enable contrasted background for non-current windows
          filetypes = {}, -- Specify which filetypes get the contrasted (darker) background
        },

        styles = { -- Give comments style such as bold, italic, underline etc.
          comments = { --[[ italic = true ]]
            italic = true,
          },
          strings = { --[[ bold = true ]]
            bold = true,
          },
          keywords = { --[[ underline = true ]]
          },
          functions = { --[[ bold = true, undercurl = true ]]
            bold = true,
          },
          variables = {},
          operators = {},
          types = {},
        },

        plugins = {
          -- "coc",
          -- "colorful-winsep",
          -- "dap",
          'dashboard',
          -- "eyeliner",
          -- "fidget",
          'flash',
          -- "gitsigns",
          'harpoon',
          'hop',
          -- "illuminate",
          'indent-blankline',
          'lspsaga',
          -- "mini",
          -- "neogit",
          -- "neotest",
          -- "neo-tree",
          -- "neorg",
          -- "noice",
          -- "nvim-cmp",
          -- "nvim-navic",
          -- "nvim-tree",
          'nvim-web-devicons',
          'rainbow-delimiters',
          'sneak',
          'telescope',
          'trouble',
          'nvim-notify',
        },

        disable = {
          colored_cursor = false, -- Disable the colored cursor
          borders = false, -- Disable borders between vertically split windows
          background = true, -- Prevent the theme from setting the background (NeoVim then uses your terminal background)
          term_colors = false, -- Prevent the theme from setting terminal colors
          eob_lines = false, -- Hide the end-of-buffer lines
        },

        high_visibility = {
          lighter = false, -- Enable higher contrast text for lighter style
          darker = false, -- Enable higher contrast text for darker style
        },

        lualine_style = 'default', -- Lualine style ( can be 'stealth' or 'default' )

        async_loading = true, -- Load parts of the theme asynchronously for faster startup (turned on by default)

        custom_colors = nil, -- If you want to override the default colors, set this to a function

        custom_highlights = {}, -- Overwrite highlights with your own
      }
      vim.g.material_style = 'deep ocean'
      vim.cmd 'colorscheme material'
    end,
  },
  -- {
  --   'catppuccin/nvim',
  --   name = 'catppuccin',
  --   priority = 1000,
  --   lazy = false,
  --   config = function()
  --     require('catppuccin').setup {
  --       flavour = 'mocha',
  --       transparent_background = true,
  --       term_colors = true,
  --       integrations = {
  --         aerial = true,
  --         alpha = true,
  --         gitsigns = true,
  --         diffview = true,
  --         blink_cmp = true,
  --         illuminate = true,
  --         indent_blankline = { enabled = true },
  --         lsp_trouble = true,
  --         mason = true,
  --         mini = {
  --           enabled = true,
  --           indentscope_color = '',
  --         },
  --         native_lsp = {
  --           enabled = true,
  --           underlines = {
  --             errors = { 'underline' },
  --             hints = { 'underline' },
  --             warnings = { 'underline' },
  --             information = { 'underline' },
  --           },
  --         },
  --         navic = { enabled = true, custom_bg = 'NONE' },
  --         neotest = true,
  --         noice = true,
  --         snacks = true,
  --         harpoon = false,
  --         notify = true,
  --         neotree = true,
  --         semantic_tokens = true,
  --         telescope = true,
  --         treesitter = true,
  --         which_key = true,
  --       },
  --       styles = {
  --         comments = {},
  --         conditionals = { 'bold' },
  --         loops = { 'bold' },
  --         functions = { 'bold' },
  --         keywords = {},
  --         strings = {},
  --         variables = {},
  --         numbers = { 'bold' },
  --         booleans = { 'bold' },
  --         properties = {},
  --         types = { 'bold' },
  --         operators = { 'bold' },
  --       },
  --       custom_highlights = function(colors)
  --         return {
  --           Comment = { fg = colors.overlay1 },
  --           LineNr = { fg = colors.overlay2 },
  --           CursorLine = { bg = colors.surface0 },
  --           CursorLineNr = { fg = colors.lavender, bold = true },
  --           Search = { bg = colors.surface1, fg = colors.text, bold = true },
  --         }
  --       end,
  --     }
  --
  --     vim.cmd.colorscheme 'catppuccin'
  --   end,
  -- },
}
