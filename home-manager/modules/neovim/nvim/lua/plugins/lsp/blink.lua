return {
  'saghen/blink.cmp',
  enabled = true,
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
      default = { 'lsp', 'path', 'snippets', 'buffer', 'dadbod', 'emoji', 'dictionary' },
      providers = {
        lsp = {
          name = 'lsp',
          enabled = true,
          module = 'blink.cmp.sources.lsp',
          min_keyword_length = 2,
          score_offset = 90,
        },
        path = {
          name = 'Path',
          module = 'blink.cmp.sources.path',
          score_offset = 25,
          fallbacks = { 'snippets', 'buffer' },
          min_keyword_length = 2,
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
          max_items = 3,
          module = 'blink.cmp.sources.buffer',
          min_keyword_length = 4,
          score_offset = 15,
        },
        dadbod = {
          name = 'Dadbod',
          module = 'vim_dadbod_completion.blink',
          min_keyword_length = 2,
          score_offset = 85,
        },
        emoji = {
          module = 'blink-emoji',
          name = 'Emoji',
          score_offset = 93,
          min_keyword_length = 2,
          opts = { insert = true },
        },
        dictionary = {
          module = 'blink-cmp-dictionary',
          name = 'Dict',
          score_offset = 20,
          enabled = true,
          max_items = 8,
          min_keyword_length = 3,
          opts = {
            dictionary_directories = { vim.fn.expand '~/github/dotfiles-latest/dictionaries' },
            dictionary_files = {
              vim.fn.expand '~/github/dotfiles-latest/neovim/neobean/spell/en.utf-8.add',
              vim.fn.expand '~/github/dotfiles-latest/neovim/neobean/spell/es.utf-8.add',
            },
          },
        },
      },
      cmdline = function()
        local type = vim.fn.getcmdtype()
        if type == '/' or type == '?' then
          return { 'buffer' }
        end
        if type == ':' then
          return { 'cmdline' }
        end
        return {}
      end,
    })

    opts.completion = {
      menu = {
        border = 'single',
      },
      documentation = {
        auto_show = true,
        window = {
          border = 'single',
        },
      },
      ghost_text = {
        enabled = true,
      },
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

-- return {
--   {
--     'saghen/blink.compat',
--     lazy = true,
--     opts = {},
--     config = function()
--       require('cmp').ConfirmBehavior = {
--         Insert = 'insert',
--         Replace = 'replace',
--       }
--     end,
--   },
--   {
--     'saghen/blink.cmp',
--     event = 'InsertEnter',
--     build = 'nix run .#build-plugin',
--     dependencies = {
--       {
--         'saghen/blink.compat',
--         optional = true,
--         opts = {},
--       },
--     },
--     opts = {
--       keymap = {
--         preset = 'default',
--         ['<C-k>'] = { 'select_prev', 'fallback' },
--         ['<C-j>'] = { 'select_next', 'fallback' },
--         ['<C-up>'] = { 'scroll_documentation_up', 'fallback' },
--         ['<C-down>'] = { 'scroll_documentation_down', 'fallback' },
--         ['<CR>'] = { 'select_and_accept', 'fallback' },
--       },
--       sources = {
--         default = { 'lsp', 'path', 'snippets', 'buffer' },
--         providers = {
--           lsp = {
--             min_keyword_length = function(ctx)
--               return ctx.trigger.kind == 'manual' and 0 or 2
--             end,
--             score_offset = 0,
--           },
--           path = {
--             min_keyword_length = 0,
--           },
--           snippets = {
--             min_keyword_length = 2,
--           },
--           buffer = {
--             min_keyword_length = 5,
--             max_items = 5,
--           },
--           avante_commands = {
--             name = 'avante_commands',
--             module = 'blink.compat.source',
--             score_offset = 90,
--             opts = {},
--           },
--           avante_files = {
--             name = 'avante_files',
--             module = 'blink.compat.source',
--             score_offset = 100,
--             opts = {},
--           },
--           avante_mentions = {
--             name = 'avante_mentions',
--             module = 'blink.compat.source',
--             score_offset = 1000,
--             opts = {},
--           },
--         },
--       },
--       completion = {
--         documentation = {
--           auto_show = true,
--           auto_show_delay_ms = 250,
--           treesitter_highlighting = true,
--           window = { border = 'rounded' },
--         },
--         list = {
--           selection = { preselect = false, auto_insert = true },
--         },
--         trigger = {
--           show_on_insert_on_trigger_character = false,
--           show_on_accept_on_trigger_character = false,
--         },
--         menu = {
--           border = 'rounded',
--           draw = {
--             columns = { { 'label', 'label_description', gap = 1 }, { 'kind_icon' } },
--           },
--         },
--       },
--       signature = { enabled = true },
--     },
--     opts_extend = {
--       'sources.default',
--       'sources.compat',
--     },
--     config = function(_, opts)
--       local enabled = opts.sources.default
--       for _, source in ipairs(opts.sources.compat or {}) do
--         opts.sources.providers[source] = vim.tbl_deep_extend('force', { name = source, module = 'blink.compat.source' }, opts.sources.providers[source] or {})
--         if type(enabled) == 'table' and not vim.tbl_contains(enabled, source) then
--           table.insert(enabled, source)
--         end
--       end
--
--       for _, provider in pairs(opts.sources.providers or {}) do
--         if provider.kind then
--           require('blink.cmp.types').CompletionItemKind[provider.kind] = provider.kind
--           local transform_items = provider.transform_items
--           provider.transform_items = function(ctx, items)
--             items = transform_items and transform_items(ctx, items) or items
--             for _, item in ipairs(items) do
--               item.kind = provider.kind or item.kind
--             end
--             return items
--           end
--         end
--       end
--
--       require('blink.cmp').setup(opts)
--     end,
--   },
-- }
