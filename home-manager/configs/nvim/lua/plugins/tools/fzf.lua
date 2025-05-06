return {
  'ibhagwan/fzf-lua',
  cmd = 'FzfLua',
  dependencies = { 'nvim-tree/nvim-web-devicons' },
  opts = {},
  config = function()
    require('fzf-lua').setup {}
  end,
}
