return {
  'richardhapb/pytest.nvim',
  dependencies = { 'nvim-treesitter/nvim-treesitter' },
  event = 'VeryLazy',
  opts = {},
  config = function(_, opts)
    require('nvim-treesitter.configs').setup {
      ensure_installed = { 'python', 'xml' },
    }

    require('pytest').setup(opts)
  end,
}
