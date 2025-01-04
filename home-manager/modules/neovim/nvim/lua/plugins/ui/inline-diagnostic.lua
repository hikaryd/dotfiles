return {
  'rachartier/tiny-inline-diagnostic.nvim',
  event = 'LspAttach',
  priority = 1000,
  config = function()
    require('tiny-inline-diagnostic').setup {
      options = {
        show_source = true,
        use_icons_from_diagnostic = true,
      },
    }
  end,
}
