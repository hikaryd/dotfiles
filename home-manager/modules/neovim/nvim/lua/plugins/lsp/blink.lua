return {
  'saghen/blink.cmp',
  lazy = false,
  build = 'nix run .#build-plugin',
  dependencies = {
    'rafamadriz/friendly-snippets',
    'L3MON4D3/LuaSnip',
    'xzbdmw/colorful-menu.nvim',
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
    signature = { enabled = true },
    completion = {
      menu = {
        draw = {
          components = {
            label = {
              width = { fill = true, max = 60 },
              text = function(ctx)
                local highlights_info = require('colorful-menu').highlights(ctx.item, vim.bo.filetype)
                if highlights_info ~= nil then
                  return highlights_info.text
                else
                  return ctx.label
                end
              end,
              highlight = function(ctx)
                local highlights_info = require('colorful-menu').highlights(ctx.item, vim.bo.filetype)
                local highlights = {}
                if highlights_info ~= nil then
                  for _, info in ipairs(highlights_info.highlights) do
                    table.insert(highlights, {
                      info.range[1],
                      info.range[2],
                      group = ctx.deprecated and 'BlinkCmpLabelDeprecated' or info[1],
                    })
                  end
                end
                for _, idx in ipairs(ctx.label_matched_indices) do
                  table.insert(highlights, { idx, idx + 1, group = 'BlinkCmpLabelMatch' })
                end
                return highlights
              end,
            },
          },
        },
      },
    },
  },
  opts_extend = { 'sources.default' },
}
