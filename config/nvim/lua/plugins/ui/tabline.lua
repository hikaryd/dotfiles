return {
  {
    'akinsho/bufferline.nvim',
    enabled = false,
    event = 'VeryLazy',
    dependencies = { 'nvim-mini/mini.icons' },
    opts = {
      options = {
        numbers = 'ordinal',
        close_command = 'bdelete! %d',
        right_mouse_command = 'bdelete! %d',
        indicator = { style = 'icon', icon = '▎' },
        separator_style = 'thick',
        show_buffer_close_icons = false,
        show_close_icon = false,
        show_tab_indicators = true,
      },
    },
  },
}
