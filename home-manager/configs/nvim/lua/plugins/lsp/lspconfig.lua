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
        pyright = { autoImportCompletion = true },
        python = { analysis = { autoSearchPaths = true, diagnosticMode = 'openFilesOnly', useLibraryCodeForTypes = true, typeCheckingMode = 'basic' } },
      },
    })

    local venv_path = os.getenv 'VIRTUAL_ENV' or ''
    local mypy_overrides = venv_path and { '--python-executable', venv_path .. '/bin/python3', true } or nil
    vim.env.PYTHONPATH = venv_path .. '/lib/python3.12/site-packages'
    setup_server('pylsp', {
      settings = {
        pyls = {
          plugins = {
            pylsp_mypy = {
              enabled = true,
              live_mode = true,
              dmypy = true,

              report_progress = true,
              strict = true,

              overrides = mypy_overrides,
            },
          },
        },
      },
    })
  end,
}
