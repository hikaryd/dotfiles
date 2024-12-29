return {
  'mvllow/modes.nvim',
  event = 'VeryLazy',
  config = function()
    require('modes').setup {
      line_opacity = 0.15,
      set_cursorline = false,
      ignore_filetypes = { 'NvimTree', 'TelescopePrompt', 'dashboard', 'minifiles' },
    }
  end,
}
