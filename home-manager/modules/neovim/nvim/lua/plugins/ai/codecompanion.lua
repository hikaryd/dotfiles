return {
  'olimorris/codecompanion.nvim',
  dependencies = {
    { 'nvim-lua/plenary.nvim' },
  },
  event = 'VeryLazy',
  opts = {
    strategies = {
      chat = { adapter = 'deepseek_r1' },
      inline = { adapter = 'mistral' },
    },
    opts = {
      log_level = 'DEBUG',
    },
  },
  config = function()
    require('codecompanion').setup {
      adapters = {
        mistral = function()
          return require('codecompanion.adapters').extend('openai_compatible', {
            env = {
              url = 'https://openrouter.ai/api/v1',
              api_key = 'OPENROUTER_API_KEY',
              chat_url = '/v1/chat/completions',
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
              chat_url = '/v1/chat/completions',
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
              chat_url = '/v1/chat/completions',
            },
            schema = {
              model = {
                default = 'deepseek/deepseek-chat',
              },
            },
          })
        end,
      },
    }
  end,
}
