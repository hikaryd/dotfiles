return {
  'saghen/blink.cmp',
  build = 'nix run .#build-plugin',
  dependencies = {
    'moyiz/blink-emoji.nvim',
    'Kaiser-Yang/blink-cmp-dictionary',
  },
  opts = function(_, opts)
    opts.enabled = function()
      local filetype = vim.bo[0].filetype
      if filetype == 'TelescopePrompt' or filetype == 'minifiles' or filetype == 'snacks_picker_input' then
        return false
      end
      return true
    end

    opts.sources = vim.tbl_deep_extend('force', opts.sources or {}, {
      default = { 'lsp', 'buffer', 'path', 'emoji', 'dictionary'},
      providers = {
        lsp = {
          name = 'LSP',
          enabled = true,
          module = 'blink.cmp.sources.lsp',
          min_keyword_length = 1,
          score_offset = 100,
        },
        path = {
          name = 'Path',
          module = 'blink.cmp.sources.path',
          min_keyword_length = 3,
          score_offset = 40,
          enabled = true,
          opts = {
            trailing_slash = false,
            label_trailing_slash = true,
            get_cwd = function(context)
              return vim.fn.expand(('#%d:p:h'):format(context.bufnr))
            end,
            show_hidden_files_by_default = true,
          },
        },
        buffer = {
          name = 'Buffer',
          enabled = true,
          max_items = 5,
          module = 'blink.cmp.sources.buffer',
          min_keyword_length = 3,
          score_offset = 50,
        },
        emoji = {
          name = 'Emoji',
          enabled = true,
          module = 'blink-emoji',
          min_keyword_length = 3,
          score_offset = 70,
          opts = { insert = true },
        },
        dictionary = {
          name = 'Dict',
          enabled = true,
          module = 'blink-cmp-dictionary',
          max_items = 5,
          min_keyword_length = 4,
          score_offset = 30,
          opts = {
            dictionary_directories = { vim.fn.expand '~/github/dotfiles-latest/dictionaries' },
            dictionary_files = {
              vim.fn.expand '~/github/dotfiles-latest/neovim/neobean/spell/en.utf-8.add',
              vim.fn.expand '~/github/dotfiles-latest/neovim/neobean/spell/es.utf-8.add',
            },
          },
        },
      },
    })

    opts.completion = {
      menu = {
        border = { '╭', '─', '╮', '│', '╯', '─', '╰', '│' },
      },
      documentation = {
        auto_show = true,
        window = {
          border = { '╭', '─', '╮', '│', '╯', '─', '╰', '│' },
        },
      },
      ghost_text = {
        enabled = false,
      },
      trigger = { prefetch_on_insert = false },
    }

    opts.keymap = {
      preset = 'default',
      ['<Tab>'] = { 'snippet_forward', 'fallback' },
      ['<S-Tab>'] = { 'snippet_backward', 'fallback' },

      ['<C-k>'] = { 'select_prev', 'fallback' },
      ['<C-j>'] = { 'select_next', 'fallback' },

      ['<S-k>'] = { 'scroll_documentation_up', 'fallback' },
      ['<S-j>'] = { 'scroll_documentation_down', 'fallback' },

      ['<C-e>'] = { 'hide', 'fallback' },

      ['<CR>'] = { 'select_and_accept', 'fallback' },
    }

    return opts
  end,
}
