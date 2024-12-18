return {
  'nvimdev/lspsaga.nvim',
  event = 'LspAttach',
  config = function()
    require('lspsaga').setup {
      ui = {
        border = 'rounded',
        title = true,
        winblend = 0,
        kind = {},
      },
      hover = {
        max_width = 0.6,
        max_height = 0.6,
        open_link = 'gx',
      },
      diagnostic = {
        on_insert = false,
        on_insert_follow = false,
        insert_winblend = 0,
        show_code_action = true,
        show_source = true,
        jump_num_shortcut = true,
        max_width = 0.7,
        max_height = 0.6,
        text_hl_follow = true,
        border_follow = true,
        keys = {
          exec_action = 'o',
          quit = 'q',
          toggle_or_jump = '<CR>',
        },
      },
      lightbulb = {
        enable = false,
      },
      symbol_in_winbar = {
        enable = true,
        separator = ' › ',
        hide_keyword = true,
        show_file = true,
        folder_level = 2,
        color_mode = true,
      },
    }

    vim.o.updatetime = 250
    vim.api.nvim_create_autocmd('CursorHold', {
      buffer = bufnr,
      callback = function()
        local opts = {
          focusable = false,
          close_events = { 'BufLeave', 'CursorMoved', 'InsertEnter', 'FocusLost' },
          border = 'rounded',
          source = 'always',
          prefix = ' ',
          scope = 'cursor',
        }
        vim.diagnostic.open_float(nil, opts)
      end,
    })
  end,
  dependencies = {
    'nvim-treesitter/nvim-treesitter',
    'nvim-tree/nvim-web-devicons',
  },
}
