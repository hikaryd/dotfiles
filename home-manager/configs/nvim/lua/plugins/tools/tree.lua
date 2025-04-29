return {
  'simonmclean/triptych.nvim',
  event = 'VeryLazy',
  dependencies = {
    'nvim-lua/plenary.nvim',
    'nvim-tree/nvim-web-devicons',
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
}
