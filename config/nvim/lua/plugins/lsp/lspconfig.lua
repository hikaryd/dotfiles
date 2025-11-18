return {
  {
    'neovim/nvim-lspconfig',
    event = { 'BufReadPre', 'BufNewFile' },
    config = function()
      local blink_cmp = require 'blink.cmp'
      local base_caps = blink_cmp.get_lsp_capabilities()

      local servers = {
        html = {},
        cssls = {},
        taplo = {},
        biome = {},
        yamlls = {},
        dockerls = {},
        jqls = {},
        nushell = {},
        nil_ls = {},
        ty = {},
        gopls = {
          settings = {
            gopls = {
              gofumpt = true,
              usePlaceholders = true,
              completeUnimported = true,
              staticcheck = true,
              analyses = {
                unusedparams = true,
                unusedwrite = true,
                nilness = true,
                shadow = true,
              },
              hints = {
                assignVariableTypes = true,
                compositeLiteralFields = true,
                compositeLiteralTypes = true,
                constantValues = true,
                functionTypeParameters = true,
                parameterNames = true,
                rangeVariableTypes = true,
              },
            },
          },
        },
        lua_ls = {
          settings = {
            Lua = {
              runtime = { version = 'LuaJIT' },
              diagnostics = { globals = { 'vim' } },
              workspace = {
                library = {
                  vim.env.VIMRUNTIME,
                  vim.fn.stdpath 'config' .. '/lua',
                },
                checkThirdParty = false,
              },
              telemetry = { enable = false },
            },
          },
        },
        ruff = {
          settings = {
            organizeImports = true,
            fixAll = true,
          },
        },
        pyright = {
          settings = {
            pyright = { autoImportCompletion = true },
            python = {
              analysis = {
                autoSearchPaths = true,
                diagnosticMode = 'openFilesOnly',
                useLibraryCodeForTypes = true,
                typeCheckingMode = 'off',
              },
            },
          },
        },
      }

      for name, cfg in pairs(servers) do
        cfg.capabilities = vim.tbl_deep_extend('force', vim.deepcopy(base_caps), cfg.capabilities or {})
        vim.lsp.config[name] = vim.tbl_deep_extend('force', vim.lsp.config[name] or {}, cfg)
      end

      for name in pairs(servers) do
        vim.lsp.enable(name)
      end

      local venv = os.getenv 'VIRTUAL_ENV' or ''
      vim.env.PYTHONPATH = venv .. '/lib/python3.12/site-packages'
    end,
  },
}
