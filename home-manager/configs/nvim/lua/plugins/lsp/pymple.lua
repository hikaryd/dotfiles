return {
  'alexpasmantier/pymple.nvim',
  dependencies = {
    'nvim-lua/plenary.nvim',
    'MunifTanjim/nui.nvim',
    'stevearc/dressing.nvim',
    'nvim-tree/nvim-web-devicons',
  },
  build = ':PympleBuild',
  event = 'VeryLazy',
  config = function()
    require('pymple').setup()
  end,
}
