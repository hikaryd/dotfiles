return {
  'nvim-lualine/lualine.nvim',
  enabled = false,
  dependencies = {
    'nvim-tree/nvim-web-devicons',
  },
  event = 'VeryLazy',
  config = function()
    require('lualine').setup {
      options = {
        component_separators = { left = '', right = '' },
        section_separators = { left = '', right = '' },
        globalstatus = true,
      },
      sections = {
        lualine_a = {
          {
            'mode',
            separator = { left = '', right = '' },
            padding = 2,
          },
        },
        lualine_b = {
          {
            'branch',
            icon = '',
            padding = { left = 2, right = 1 },
          },
          {
            'diff',
            symbols = {
              added = ' ',
              modified = ' ',
              removed = ' ',
            },
          },
        },
        lualine_c = {
          {
            'filename',
            path = 1,
            symbols = {
              modified = '●',
              readonly = '',
              unnamed = '[No Name]',
            },
          },
          {
            'diagnostics',
            sources = { 'nvim_diagnostic' },
            symbols = {
              error = ' ',
              warn = ' ',
              info = ' ',
              hint = ' ',
            },
          },
        },
        lualine_x = {
          {
            'filetype',
            colored = true,
            icon_only = false,
          },
          'encoding',
          'fileformat',
        },
        lualine_y = { 'progress' },
        lualine_z = {
          {
            'location',
            separator = { left = '', right = '' },
            padding = 2,
          },
        },
      },
      extensions = {
        'neo-tree',
        'lazy',
        'mason',
        'trouble',
      },
    }
  end,
}
