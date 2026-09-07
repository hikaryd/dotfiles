return {
  'MeanderingProgrammer/render-markdown.nvim',
  ft = { 'markdown', 'Avante' },
  dependencies = { 'nvim-treesitter/nvim-treesitter', 'nvim-mini/mini.icons' },
  config = function()
    require('render-markdown').setup {}
  end,
}
