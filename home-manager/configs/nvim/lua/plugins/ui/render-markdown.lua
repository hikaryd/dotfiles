return {
  'MeanderingProgrammer/render-markdown.nvim',
  ft = { 'markdown', 'Avante' },
  dependencies = { 'nvim-treesitter/nvim-treesitter', 'nvim-tree/nvim-web-devicons' },
  config = function()
    require('render-markdown').setup {}
  end,
}
