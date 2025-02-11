-- Возвращаем список всех плагинов
return {
  -- Core plugins
  { import = "plugins.core" },
  -- UI plugins
  { import = "plugins.ui" },
  -- Editor plugins
  { import = "plugins.editor" },
  -- LSP plugins
  { import = "plugins.lsp" },
  -- Tools
  { import = "plugins.tools" },
  -- AI plugins
  { import = "plugins.ai" },
}
