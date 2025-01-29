return {
  {
    'tadaa/vimade',
    event = 'VeryLazy',
    opts = {
      recipe = { 'default', { animate = true } },
      ncmode = 'buffers',
      fadelevel = 0.4, -- any value between 0 and 1. 0 is hidden and 1 is opaque.
      -- Changes the real or theoretical background color. basebg can be used to give
      -- transparent terminals accurating dimming.  See the 'Preparing a transparent terminal'
      -- section in the README.md for more info.
      -- basebg = [23,23,23],
      basebg = '',
      tint = {
        bg = { rgb = { 0, 0, 0 }, intensity = 0.3 }, -- adds 30% black to background
        fg = { rgb = { 0, 0, 255 }, intensity = 0.3 }, -- adds 30% blue to foreground
        -- fg = { rgb = { 120, 120, 120 }, intensity = 1 }, -- all text will be gray
        sp = { rgb = { 255, 0, 0 }, intensity = 0.5 }, -- adds 50% red to special characters
      },
      blocklist = {
        default = {
          highlights = {
            laststatus_3 = function(win, active)
              if vim.go.laststatus == 3 then
                return 'StatusLine'
              end
            end,
            'TabLineSel',
            'Pmenu',
            'PmenuSel',
            'PmenuKind',
            'PmenuKindSel',
            'PmenuExtra',
            'PmenuExtraSel',
            'PmenuSbar',
            'PmenuThumb',
          },
          buf_opts = { buftype = { 'prompt' } },
        },
        default_block_floats = function(win, active)
          return win.win_config.relative ~= '' and (win ~= active or win.buf_opts.buftype == 'terminal') and true or false
        end,
      },
      -- Link connects windows so that they style or unstyle together.
      -- Properties are matched against the active window. Same format as blocklist above
      link = {},
      groupdiff = true, -- links diffs so that they style together
      groupscrollbind = false, -- link scrollbound windows so that they style together.
      -- enable to bind to FocusGained and FocusLost events. This allows fading inactive
      -- tmux panes.
      enablefocusfading = false,
      -- Time in milliseconds before re-checking windows. This is only used when usecursorhold
      -- is set to false.
      checkinterval = 1000,
      -- enables cursorhold event instead of using an async timer.  This may make Vimade
      -- feel more performant in some scenarios. See h:updatetime.
      usecursorhold = false,
      nohlcheck = true,
      focus = {
        providers = {
          filetypes = {
            default = {
              { 'snacks', {} },
              { 'mini', {} },
              {
                'treesitter',
                {
                  min_node_size = 2,
                  min_size = 1,
                  max_size = 0,
                  exclude = {
                    'script_file',
                    'stream',
                    'document',
                    'source_file',
                    'translation_unit',
                    'chunk',
                    'module',
                    'stylesheet',
                    'statement_block',
                    'block',
                    'pair',
                    'program',
                    'switch_case',
                    'catch_clause',
                    'finally_clause',
                    'property_signature',
                    'dictionary',
                    'assignment',
                    'expression_statement',
                    'compound_statement',
                  },
                },
              },
              { 'blanks', {
                min_size = 1,
                max_size = '35%',
              } },
              { 'static', {
                size = '35%',
              } },
            },
            -- markdown ={{'blanks', {min_size=0, max_size='50%'}}, {'static', {max_size='50%'}}}
            -- javascript = {
            -- -- only use treesitter (no fallbacks)
            --   {'treesitter', { min_node_size = 2, include = {'if_statement', ...}}},
            -- },
            -- typescript = {
            --   {'treesitter', { min_node_size = 2, exclude = {'if_statement'}}},
            --   {'static', {size = '35%'}}
            -- },
            -- java = {
            -- -- mini with a fallback to blanks
            -- {'mini', {min_size = 1, max_size = 20}},
            -- {'blanks', {min_size = 1, max_size = '100%' }},
            -- },
          },
        },
      },
    },
  },
}
