return {
  'stevearc/aerial.nvim',
  event = 'VeryLazy',
  opts = {
    attach_mode = 'global',
    backends = { 'lsp', 'treesitter', 'markdown', 'man' },
    layout = { min_width = 28 },
    show_guides = true,
    filter_kind = false,
    guides = {
      mid_item = '├ ',
      last_item = '└ ',
      nested_top = '│ ',
      whitespace = '  ',
    },
    keymaps = {
      ['<CR>'] = 'actions.jump',
      ['<2-LeftMouse>'] = 'actions.jump',
      ['<C-v>'] = 'actions.jump_vsplit',
      ['<C-s>'] = 'actions.jump_split',
      ['p'] = 'actions.scroll',
      ['<C-j>'] = 'actions.down_and_scroll',
      ['<C-k>'] = 'actions.up_and_scroll',
    },
  },
  config = function()
    require('aerial').setup {
      on_attach = function(bufnr)
        vim.keymap.set('n', '{', '<cmd>AerialPrev<CR>', { buffer = bufnr })
        vim.keymap.set('n', '}', '<cmd>AerialNext<CR>', { buffer = bufnr })
      end,
    }
  end,
}
