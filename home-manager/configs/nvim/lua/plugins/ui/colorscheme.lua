return {
  {
    'scottmckendry/cyberdream.nvim',
    lazy = false,
    config = function()
      require('cyberdream').setup {
        variant = 'default', -- use "light" for the light variant. Also accepts "auto" to set dark or light colors based on the current value of `vim.o.background`
        transparent = true,
        saturation = 1, -- accepts a value between 0 and 1. 0 will be fully desaturated (greyscale) and 1 will be the full color (default)
        italic_comments = true,
        hide_fillchars = true,
        borderless_pickers = true,
        terminal_colors = true,
        extensions = {
          telescope = true,
          notify = true,
          mini = true,
          blinkcmp = true,
          fzflua = true,
          lazy = true,
          markdown = true,
        },
      }
    end,
  },
  {
    'catppuccin/nvim',
    name = 'catppuccin',
    lazy = false,
    config = function()
      require('catppuccin').setup {
        flavour = 'mocha',
        transparent_background = true,
        term_colors = true,
        integrations = {
          aerial = true,
          alpha = true,
          gitsigns = true,
          diffview = true,
          blink_cmp = true,
          illuminate = true,
          indent_blankline = { enabled = true },
          lsp_trouble = true,
          mason = true,
          mini = {
            enabled = true,
            indentscope_color = '',
          },
          native_lsp = {
            enabled = true,
            underlines = {
              errors = { 'underline' },
              hints = { 'underline' },
              warnings = { 'underline' },
              information = { 'underline' },
            },
          },
          navic = { enabled = true, custom_bg = 'NONE' },
          neotest = true,
          noice = true,
          snacks = true,
          harpoon = false,
          notify = true,
          neotree = true,
          semantic_tokens = true,
          telescope = true,
          treesitter = true,
          which_key = true,
        },
        styles = {
          comments = {},
          conditionals = { 'bold' },
          loops = { 'bold' },
          functions = { 'bold' },
          keywords = {},
          strings = {},
          variables = {},
          numbers = { 'bold' },
          booleans = { 'bold' },
          properties = {},
          types = { 'bold' },
          operators = { 'bold' },
        },
        custom_highlights = function(colors)
          return {
            Comment = { fg = colors.overlay1 },
            LineNr = { fg = colors.overlay2 },
            CursorLine = { bg = colors.surface0 },
            CursorLineNr = { fg = colors.lavender, bold = true },
            Search = { bg = colors.surface1, fg = colors.text, bold = true },
          }
        end,
      }

      -- vim.cmd.colorscheme 'catppuccin'
    end,
  },
}
