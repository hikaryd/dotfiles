return {
  'ibhagwan/fzf-lua',
  cmd = 'FzfLua',
  dependencies = { 'echasnovski/mini.icons' },
  opts = {},
  config = function()
    require('fzf-lua').setup {}
  end,
}
