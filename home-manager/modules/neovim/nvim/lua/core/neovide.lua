local alpha = function()
  return string.format('%x', math.floor(255 * vim.g.neovide_transparency_point or 0.8))
end
vim.g.neovide_transparency = 0.66
vim.g.neovide_transparency_point = 0.66
vim.g.neovide_background_color = '#0E0F12' .. alpha()
local change_transparency = function(delta)
  vim.g.neovide_transparency_point = vim.g.neovide_transparency_point + delta
  vim.g.neovide_background_color = '#0E0F12' .. alpha()
end
vim.keymap.set({ 'n', 'v', 'o' }, '<D-]>', function()
  change_transparency(0.01)
end)
vim.keymap.set({ 'n', 'v', 'o' }, '<D-[>', function()
  change_transparency(-0.01)
end)
