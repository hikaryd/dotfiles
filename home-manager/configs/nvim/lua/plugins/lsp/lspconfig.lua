return {
  'neovim/nvim-lspconfig',
  event = { 'BufReadPre', 'BufNewFile' },
  ft = { 'nix' },
  config = function()
    local lspconfig = require 'lspconfig'

    local function setup_server(server, config)
      if not config then
        return
      end

      if type(config) ~= 'table' then
        config = {}
      end

      config = vim.tbl_deep_extend('force', {
        capabilities = require('blink.cmp').get_lsp_capabilities(config.capabilities),
      }, config)

      lspconfig[server].setup(config)
    end

    local servers = {
      'html',
      'cssls',
      'lua_ls',
      'bashls',
      'taplo',
      'biome',
      'yamlls',
      'dockerls',
      'jqls',
      'nushell',
      'nil_ls',
    }

    for _, server in ipairs(servers) do
      setup_server(server, {})
    end

    setup_server('lua_ls', {
      settings = {
        Lua = {
          runtime = { version = 'LuaJIT' },
          diagnostics = { globals = { 'vim' } },
          workspace = {
            library = {
              vim.env.VIMRUNTIME,
              "${vim.fn.stdpath('config')}/lua",
            },
            checkThirdParty = false,
          },
          telemetry = { enable = false },
        },
      },
    })

    setup_server('ruff', {
      settings = {
        organizeImports = true,
        fixAll = true,
      },
    })

    setup_server('pyright', {
      settings = {
        pyright = {
          analysis = {
            ignore = { '*' },
            typeCheckingMode = 'off',
            diagnosticMode = 'openFilesOnly',
            useLibraryCodeForTypes = true,
            reportMissingImports = false,
            reportUndefinedVariable = false,
          },
        },
      },
      on_attach = function(client, bufnr)
        if client.name == 'pyright' then
          client.handlers['textDocument/publishDiagnostics'] = function() end
        end
      end,
    })

    setup_server('pylsp', {
      settings = {
        pyls = {
          plugins = {
            jedi_completion = { enabled = false },
            jedi_hover = { enabled = false },
            jedi_references = { enabled = false },
            jedi_signature_help = { enabled = false },
            jedi_symbols = { enabled = false },
            pycodestyle = { enabled = false },
            flake8 = {
              enabled = false,
              ignore = {},
              maxLineLength = 160,
            },
            mypy = { enabled = false },
            isort = { enabled = false },
            yapf = { enabled = false },
            pylint = { enabled = false },
            pydocstyle = { enabled = false },
            mccabe = { enabled = false },
            preload = { enabled = false },
            rope_completion = { enabled = false },
            pylsp_mypy = {
              enabled = true,
              live_mode = true,
              report_progress = true,
            },
          },
        },
      },
    })

    vim.o.updatetime = 250
    vim.api.nvim_create_autocmd('CursorHold', {
      callback = function()
        local diag = vim.diagnostic.get(0, { lnum = vim.fn.line '.' - 1 })
        if #diag > 0 then
          vim.diagnostic.open_float {
            scope = 'line',
            border = 'rounded',
            focusable = false,
          }
        end
      end,
    })
  end,
  dependencies = {
    'saghen/blink.cmp',
  },
}
