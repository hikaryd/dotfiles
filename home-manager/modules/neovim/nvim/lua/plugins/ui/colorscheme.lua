return {
  {
    'catppuccin/nvim',
    name = 'catppuccin',
    priority = 1000,
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
          illuminate = true,
          indent_blankline = { enabled = true },
          lsp_trouble = true,
          mason = true,
          mini = true,
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

      vim.cmd.colorscheme 'catppuccin'
    end,
  },
}
