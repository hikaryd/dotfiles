return {
  'mvllow/modes.nvim',
  event = 'VeryLazy',
  config = function()
    require('modes').setup {
      line_opacity = 0.30,
      set_cursorline = true,
      ignore_filetypes = { 'NvimTree', 'TelescopePrompt', 'dashboard', 'minifiles' },
    }
  end,
}
