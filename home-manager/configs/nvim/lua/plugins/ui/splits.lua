return {
  {
    'mrjones2014/smart-splits.nvim',
    event = 'VeryLazy',
    opts = {
      ignored_filetypes = { 'nofile', 'quickfix', 'prompt', 'toggleterm' },
      resize_mode = { silent = true },
    },
    config = function(_, opts)
      require('smart-splits').setup(opts)
    end,
  },
}
