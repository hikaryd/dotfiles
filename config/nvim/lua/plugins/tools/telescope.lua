return {
  'nvim-telescope/telescope.nvim',
  cmd = 'Telescope',
  version = false,
  dependencies = {
    'nvim-lua/plenary.nvim',
    {
      'nvim-telescope/telescope-fzf-native.nvim',
      build = 'make',
      enabled = vim.fn.executable 'make' == 1,
    },
    'nvim-telescope/telescope-ui-select.nvim',
    'echasnovski/mini.icons',
  },
  config = function()
    local telescope = require 'telescope'
    local actions = require 'telescope.actions'
    local themes = require 'telescope.themes'

    telescope.setup {
      defaults = {
        prompt_prefix = '  ',
        selection_caret = ' ',
        entry_prefix = ' ',
        multi_icon = ' ',
        initial_mode = 'insert',
        selection_strategy = 'reset',
        path_display = { 'truncate' },
        color_devicons = true,
        vimgrep_arguments = {
          'rg',
          '--color=never',
          '--no-heading',
          '--with-filename',
          '--line-number',
          '--column',
          '--smart-case',
          '--hidden',
          '--glob=!.git/',
        },
        set_env = { ['COLORTERM'] = 'truecolor' },
        sorting_strategy = 'ascending',
        layout_config = {
          horizontal = {
            prompt_position = 'top',
            preview_width = 0.55,
            results_width = 0.8,
          },
          vertical = {
            mirror = false,
          },
          width = 0.87,
          height = 0.80,
          preview_cutoff = 120,
        },
        border = true,
        borderchars = { '─', '│', '─', '│', '╭', '╮', '╯', '╰' },
        file_sorter = require('telescope.sorters').get_fuzzy_file,
        generic_sorter = require('telescope.sorters').get_generic_fuzzy_sorter,
        file_ignore_patterns = {
          'node_modules',
          '.git/',
          '.cache',
          '__pycache__',
          '%.pyc',
          '.pytest_cache',
          '.mypy_cache',
          '.ruff_cache',
          '.coverage',
          '%.o',
          '%.a',
          '%.out',
          '%.class',
          '%.pdf',
          '%.mkv',
          '%.mp4',
          '%.zip',
        },
        mappings = {
          i = {
            ['<C-n>'] = actions.cycle_history_next,
            ['<C-p>'] = actions.cycle_history_prev,
            ['<C-j>'] = actions.move_selection_next,
            ['<C-k>'] = actions.move_selection_previous,
            ['<C-c>'] = actions.close,
            ['<CR>'] = actions.select_default,
            ['<C-s>'] = actions.select_horizontal,
            ['<C-v>'] = actions.select_vertical,
            ['<C-t>'] = actions.select_tab,
            ['<C-u>'] = actions.preview_scrolling_up,
            ['<C-d>'] = actions.preview_scrolling_down,
            ['<C-q>'] = actions.send_to_qflist + actions.open_qflist,
            ['<M-q>'] = actions.send_selected_to_qflist + actions.open_qflist,
          },
          n = {
            ['<esc>'] = actions.close,
            ['q'] = actions.close,
            ['<CR>'] = actions.select_default,
            ['<C-x>'] = actions.select_horizontal,
            ['<C-v>'] = actions.select_vertical,
            ['<C-t>'] = actions.select_tab,
            ['<C-q>'] = actions.send_to_qflist + actions.open_qflist,
            ['<M-q>'] = actions.send_selected_to_qflist + actions.open_qflist,
            ['j'] = actions.move_selection_next,
            ['k'] = actions.move_selection_previous,
            ['H'] = actions.move_to_top,
            ['M'] = actions.move_to_middle,
            ['L'] = actions.move_to_bottom,
            ['gg'] = actions.move_to_top,
            ['G'] = actions.move_to_bottom,
          },
        },
      },
      pickers = {
        find_files = {
          theme = 'dropdown',
          previewer = false,
          hidden = true,
          find_command = { 'rg', '--files', '--hidden', '--glob', '!**/.git/*' },
        },
        live_grep = {
          theme = 'dropdown',
          previewer = true,
        },
        buffers = {
          theme = 'dropdown',
          previewer = false,
        },
        help_tags = {
          theme = 'dropdown',
          previewer = true,
        },
        planets = {
          show_pluto = true,
          show_moon = true,
        },
        git_files = {
          theme = 'dropdown',
          previewer = false,
          show_untracked = true,
        },
        git_branches = {
          theme = 'dropdown',
          previewer = false,
        },
      },
      extensions = {
        fzf = {
          fuzzy = true,
          override_generic_sorter = true,
          override_file_sorter = true,
          case_mode = 'smart_case',
        },
        ['ui-select'] = {
          themes.get_dropdown {
            initial_mode = 'normal',
          },
        },
      },
    }

    telescope.load_extension 'fzf'
    telescope.load_extension 'ui-select'
  end,
}
