return {
  {
    event = 'VeryLazy',
    'echasnovski/mini.surround',
    config = function()
      require('mini.surround').setup()
    end,
  },
  {
    'echasnovski/mini.indentscope',
    event = { 'BufReadPre', 'BufNewFile' },
    opts = {
      symbol = '│',
      options = { try_as_border = true },
    },
  },
  {
    'echasnovski/mini.files',
    version = '*',
    event = 'VeryLazy',
    config = function()
      require('mini.files').setup()
    end,
  },
}
