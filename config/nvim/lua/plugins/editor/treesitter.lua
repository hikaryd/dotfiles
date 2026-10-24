return {
  'nvim-treesitter/nvim-treesitter',
  event = 'VeryLazy',
  cmd = {
    'TSInstall',
    'TSBufEnable',
    'TSBufDisable',
    'TSModuleInfo',
  },
  build = ':TSUpdate',
  config = function()
    require('nvim-treesitter.configs').setup {
      ensure_installed = {
        'vim',
        'lua',
        'vimdoc',
        'html',
        'css',
        'python',
        'hurl',
        'javascript',
        'typescript',
        'rust',
        'go',
        'bash',
        'markdown',
        'markdown_inline',
        'dockerfile',
      },

      auto_install = true,

      highlight = {
        enable = true,
        additional_vim_regex_highlighting = false,
      },

      indent = { enable = true },

      incremental_selection = {
        enable = true,
        keymaps = {
          init_selection = '<CR>',
          node_incremental = '<CR>',
          scope_incremental = '<S-CR>',
          node_decremental = '<BS>',
        },
      },
      textobjects = {
        select = {
          enable = true,
          lookahead = true,
          keymaps = {
            ['af'] = '@function.outer',
            ['if'] = '@function.inner',
            ['ac'] = '@class.outer',
            ['ic'] = '@class.inner',
            ['a='] = '@assignment.outer',
            ['i='] = '@assignment.inner',
          },
        },
        move = {
          enable = true,
          set_jumps = true,
          goto_next_start = {
            [']m'] = '@function.outer',
            [']]'] = '@class.outer',
          },
          goto_previous_start = {
            ['[m'] = '@function.outer',
            ['[['] = '@class.outer',
          },
        },
      },
    }
  end,
  dependencies = {
    'nvim-treesitter/nvim-treesitter-textobjects',
  },
}
