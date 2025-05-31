return {
  'frankroeder/parrot.nvim',
  lazy = true,
  event = 'VeryLazy',
  dependencies = {
    'ibhagwan/fzf-lua',
    'nvim-lua/plenary.nvim',
  },
  config = function()
    require('parrot').setup {
      providers = {
        gemini = {
          api_key = { 'cat', '~/.gemini_key' },
          topic = {
            model = 'gemini-2.5-flash-preview-05-20',
          },
        },
      },
      online_model_selection = false,
    }
  end,
}
