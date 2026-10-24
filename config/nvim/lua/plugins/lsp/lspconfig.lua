return {
  {
    'neovim/nvim-lspconfig',
    event = { 'BufReadPre', 'BufNewFile' },
    dependencies = { 'saghen/blink.cmp' },
    config = function()
      local blink_cmp = require('blink.cmp')
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
        -- ty — единственный Python IDE-provider; Ruff отвечает за lint/format.
        ty = {
          settings = {
            ty = {
              diagnosticMode = 'openFilesOnly',
              showSyntaxErrors = false,
              completions = { autoImport = true },
            },
          },
        },
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
                  vim.fn.stdpath('config') .. '/lua',
                },
                checkThirdParty = false,
              },
              telemetry = { enable = false },
            },
          },
        },
        ruff = {
          init_options = {
            settings = { organizeImports = true, fixAll = true },
          },
          on_attach = function(client)
            -- Hover приходит от ty, без второго пустого ответа Ruff.
            client.server_capabilities.hoverProvider = false
          end,
        },
      }

      for name, cfg in pairs(servers) do
        cfg.capabilities = vim.tbl_deep_extend('force', vim.deepcopy(base_caps), cfg.capabilities or {})
        vim.lsp.config[name] = vim.tbl_deep_extend('force', vim.lsp.config[name] or {}, cfg)
      end

      for name in pairs(servers) do
        vim.lsp.enable(name)
      end

      -- Не переопределяем PYTHONPATH: ty определяет окружение проекта сам.
    end,
  },
}
