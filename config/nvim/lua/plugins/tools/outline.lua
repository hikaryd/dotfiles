return {
  'hedyhli/outline.nvim',
  cmd = {
    'Outline',
    'OutlineOpen',
    'OutlineClose',
    'OutlineFocusOutline',
    'OutlineFocusCode',
    'OutlineFocus',
    'OutlineStatus',
    'OutlineFollow',
    'OutlineRefresh',
  },
  config = function()
    require('outline').setup({})
  end,
}
