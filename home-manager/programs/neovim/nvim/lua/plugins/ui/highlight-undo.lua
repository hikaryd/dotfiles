return {
  'zeioth/highlight-undo.nvim',
  event = 'VeryLazy',
  opts = {
    duration = 150,
    redo = { hlgroup = 'IncSearch' },
  },
  config = function(_, opts)
    require('highlight-undo').setup(opts)

    vim.api.nvim_create_autocmd('TextYankPost', {
      desc = 'Highlight yanked text',
      pattern = '*',
      callback = function()
        (vim.hl or vim.highlight).on_yank()
      end,
    })
  end,
}
