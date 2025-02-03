return {
  'olimorris/codecompanion.nvim',
  dependencies = {
    { 'nvim-lua/plenary.nvim' },
  },
  enabled = false,
  event = 'VeryLazy',
  config = function()
    require('codecompanion').setup {
      strategies = {
        chat = { adapter = 'deepseek' },
        inline = { adapter = 'mistral' },
      },
      adapters = {
        google = function()
          return require('codecompanion.adapters').extend('openai_compatible', {
            env = {
              url = 'https://openrouter.ai/api/v1',
              api_key = 'OPENROUTER_API_KEY',
              chat_url = '/chat/completions',
            },
            schema = {
              model = {
                default = 'google/gemini-2.0-flash-thinking-exp-1219:free',
              },
            },
          })
        end,
        openai = function()
          return require('codecompanion.adapters').extend('openai_compatible', {
            env = {
              url = 'https://openrouter.ai/api/v1',
              api_key = 'OPENROUTER_API_KEY',
              chat_url = '/chat/completions',
            },
            schema = {
              model = {
                default = 'openai/gpt-4o-2024-11-20',
              },
            },
          })
        end,
        mistral = function()
          return require('codecompanion.adapters').extend('openai_compatible', {
            env = {
              url = 'https://openrouter.ai/api/v1',
              chat_url = '/chat/completions',
              api_key = 'OPENROUTER_API_KEY',
            },
            schema = {
              model = {
                default = 'mistralai/codestral-2501',
              },
            },
          })
        end,
        deepseek_r1 = function()
          return require('codecompanion.adapters').extend('openai_compatible', {
            env = {
              url = 'https://openrouter.ai/api/v1',
              api_key = 'OPENROUTER_API_KEY',
              chat_url = '/chat/completions',
            },
            schema = {
              model = {
                default = 'deepseek/deepseek-r1',
              },
            },
          })
        end,
        deepseek = function()
          return require('codecompanion.adapters').extend('openai_compatible', {
            env = {
              url = 'https://openrouter.ai/api/v1',
              api_key = 'OPENROUTER_API_KEY',
              chat_url = '/chat/completions',
            },
            schema = {
              model = {
                default = 'deepseek/deepseek-chat',
              },
            },
          })
        end,
      },
      display = {
        chat = {
          window = {
            layout = 'vertical', -- float|vertical|horizontal|buffer
            position = 'right',
            border = 'single',
            height = 0.8,
            width = 0.3,
            relative = 'editor',
            opts = {
              breakindent = true,
              cursorcolumn = false,
              cursorline = false,
              foldcolumn = '0',
              linebreak = true,
              list = false,
              numberwidth = 1,
              signcolumn = 'no',
              spell = false,
              wrap = true,
            },
          },
        },
      },
    }
  end,
}
