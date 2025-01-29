-- Initialize core configuration
local M = {}

function M.setup()
  -- Load core modules
  require 'core.options'
  require 'core.mappings'
  require 'core.neovide'

  -- Setup autocmds if they exist
  local ok, _ = pcall(require, 'core.autocmds')
  if ok then
    require('core.autocmds').setup()
  end
end

return M
