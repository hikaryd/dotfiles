return {
  {
    'supermaven-inc/supermaven-nvim',
    enabled = false,
    event = 'VeryLazy',
    config = function()
      require('supermaven-nvim').setup {}
    end,
  },
}
