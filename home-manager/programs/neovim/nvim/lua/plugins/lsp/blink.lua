return {
  'saghen/blink.cmp',
  lazy = false,
  build = 'nix run .#build-plugin',
  dependencies = {
    'rafamadriz/friendly-snippets',
    'L3MON4D3/LuaSnip',
  },
  opts = {
    fuzzy = {},
    keymap = {
      preset = 'default',
      ['<C-k>'] = { 'select_prev', 'fallback' },
      ['<C-j>'] = { 'select_next', 'fallback' },
      ['<CR>'] = { 'select_and_accept', 'fallback' },
    },
    appearance = {
      use_nvim_cmp_as_default = true,
      nerd_font_variant = 'mono',
    },
    sources = {
      default = { 'lsp', 'path', 'luasnip', 'buffer' },
    },
    snippets = {
      expand = function(snippet)
        require('luasnip').lsp_expand(snippet)
      end,
      active = function(filter)
        if filter and filter.direction then
          return require('luasnip').jumpable(filter.direction)
        end
        return require('luasnip').in_snippet()
      end,
      jump = function(direction)
        require('luasnip').jump(direction)
      end,
    },
    completion = {
      menu = {
        draw = {
          columns = {
            { 'kind_icon' },
            { 'label', 'label_description', gap = 1 },
          },
          components = {
            item_idx = {
              text = function(ctx)
                return tostring(ctx.idx)
              end,
              highlight = 'BlinkCmpItemIdx',
            },
          },
        },
      },
    },
    signature = { enabled = true },
    documentation = {
      border = vim.g.borderStyle,
      min_width = 15,
      max_width = 45,
      max_height = 10,
      auto_show = true,
      auto_show_delay_ms = 250,
    },
    autocomplete = {
      border = vim.g.borderStyle,
      min_width = 10, -- max_width controlled by draw-function
      max_height = 10,
      cycle = { from_top = false }, -- cycle at bottom, but not at the top
      draw = function(ctx)
        local source, client = ctx.item.source_id, ctx.item.client_id
        if client and vim.lsp.get_client_by_id(client).name == 'emmet_language_server' then
          source = 'emmet'
        end

        local sourceIcons = { snippets = '󰩫', buffer = '󰦨', emmet = '' }
        local icon = sourceIcons[source] or ctx.kind_icon

        return {
          {
            ' ' .. ctx.item.label .. ' ',
            fill = true,
            hl_group = ctx.deprecated and 'BlinkCmpLabelDeprecated' or 'BlinkCmpLabel',
            max_width = 40,
          },
          { icon .. ' ', hl_group = 'BlinkCmpKind' .. ctx.kind },
        }
      end,
    },
  },
  opts_extend = { 'sources.default' },
}
