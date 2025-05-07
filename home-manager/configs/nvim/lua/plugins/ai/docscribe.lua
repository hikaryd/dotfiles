return {
  'AdrianMosnegutu/docscribe.nvim',
  event = 'VeryLazy',
  config = function()
    require('docscribe').setup {
      llm = {
        provider = 'ollama',
        model = 'qwen3:4b',
      },
    }
  end,
}
