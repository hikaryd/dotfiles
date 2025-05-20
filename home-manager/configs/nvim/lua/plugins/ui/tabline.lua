return {
  {
    'akinsho/bufferline.nvim',
    event = 'VeryLazy',
    dependencies = { 'nvim-tree/nvim-web-devicons' },
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
