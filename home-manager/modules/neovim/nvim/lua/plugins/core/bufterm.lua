return {
  'boltlessengineer/bufterm.nvim',
  event = 'VeryLazy',
  config = function()
    require('bufterm').setup()
  end,
}
