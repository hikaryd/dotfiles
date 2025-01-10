return {
  {
    'saghen/blink.cmp',
    event = 'InsertEnter',
    build = 'nix run .#build-plugin',
    dependencies = {
      {
        'saghen/blink.compat',
        optional = true,
        opts = {},
      },
    },
    opts = {
      fuzzy = {},
      keymap = {
        preset = 'default',
        ['<C-k>'] = { 'select_prev', 'fallback' },
        ['<C-j>'] = { 'select_next', 'fallback' },
        ['<C-up>'] = { 'scroll_documentation_up', 'fallback' },
        ['<C-down>'] = { 'scroll_documentation_down', 'fallback' },
        ['<CR>'] = { 'select_and_accept', 'fallback' },
      },
      sources = {
        default = { 'lsp', 'path', 'snippets', 'buffer' },
        providers = {
          lsp = {
            min_keyword_length = function(ctx)
              return ctx.trigger.kind == 'manual' and 0 or 2 -- trigger when invoking with shortcut
            end,
            score_offset = 0,
          },
          path = {
            min_keyword_length = 0,
          },
          snippets = {
            min_keyword_length = 2,
          },
          buffer = {
            min_keyword_length = 5,
            max_items = 5,
          },
        },
      },
      completion = {
        documentation = {
          auto_show = true,
          auto_show_delay_ms = 250,
          treesitter_highlighting = true,
          window = { border = 'rounded' },
        },
        list = {
          selection = { preselect = false, auto_insert = true },
        },
        trigger = {
          show_on_insert_on_trigger_character = false,
          show_on_accept_on_trigger_character = false,
        },
        menu = {
          border = 'rounded',
          draw = {
            columns = { { 'label', 'label_description', gap = 1 }, { 'kind_icon' } },
          },
        },
      },
      -- completion = { accept = { auto_brackets = { enabled = true } } },
      signature = { enabled = true },
    },
    opts_extend = {
      'sources.default',
      'sources.compat',
    },
    config = function(_, opts)
      local enabled = opts.sources.default
      for _, source in ipairs(opts.sources.compat or {}) do
        opts.sources.providers[source] = vim.tbl_deep_extend('force', { name = source, module = 'blink.compat.source' }, opts.sources.providers[source] or {})
        if type(enabled) == 'table' and not vim.tbl_contains(enabled, source) then
          table.insert(enabled, source)
        end
      end

      for _, provider in pairs(opts.sources.providers or {}) do
        if provider.kind then
          require('blink.cmp.types').CompletionItemKind[provider.kind] = provider.kind
          local transform_items = provider.transform_items
          provider.transform_items = function(ctx, items)
            items = transform_items and transform_items(ctx, items) or items
            for _, item in ipairs(items) do
              item.kind = provider.kind or item.kind
            end
            return items
          end
        end
      end

      require('blink.cmp').setup(opts)
    end,
  },
  {
    'saghen/blink.cmp',
    opts = {
      sources = {
        default = { 'lazydev' },
        providers = {
          lsp = {
            fallbacks = { 'buffer' },
          },
          lazydev = {
            name = 'LazyDev',
            module = 'lazydev.integrations.blink',
          },
        },
      },
    },
  },
  {
    'folke/lazydev.nvim',
    ft = 'lua',
    opts = {
      library = {
        { path = '${3rd}/luv/library', words = { 'vim%.uv' } },
      },
    },
  },
}
