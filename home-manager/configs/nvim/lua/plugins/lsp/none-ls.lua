return {
  'nvimtools/none-ls.nvim',
  dependencies = { 'nvim-lua/plenary.nvim' },
  event = 'BufReadPre',
  config = function()
    local null_ls = require 'null-ls'

    null_ls.setup {
      sources = {
        null_ls.builtins.diagnostics.mypy.with {
          extra_args = function()
            local virtual = os.getenv 'VIRTUAL_ENV' or vim.fn.getcwd() .. '/.venv'
            return { '--python-executable', virtual .. '/bin/python' }
          end,
        },
      },
    }
  end,
}
