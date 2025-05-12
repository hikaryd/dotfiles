return {
  'AdrianMosnegutu/docscribe.nvim',
  event = 'VeryLazy',
  config = function()
    require('docscribe').setup {
      llm = {
        provider = 'ollama',
        model = 'slimsag/starchat2:15b-v0.1-f16-q4_0',
      },
    }
  end,
}
