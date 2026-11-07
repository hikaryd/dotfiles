local expected = {
  "#181818", "#ff8080", "#50b5aa", "#ffc799", "#b0b0b0", "#ffc799", "#50b5aa", "#dadada",
  "#505050", "#ff8080", "#99ffe4", "#ffcfa8", "#b0b0b0", "#ffcfa8", "#99ffe4", "#ffffff",
}
local function check()
  for i, color in ipairs(expected) do
    assert(vim.g["terminal_color_" .. (i - 1)] == color, "terminal color " .. (i - 1))
  end
  assert(vim.g.terminal_color_background == "#101010")
  assert(vim.g.terminal_color_foreground == "#ffffff")
end

local function configure()
  dofile("config/nvim/lua/plugins/ui/colorscheme.lua")[1].config()
end

configure()
check()
vim.cmd.colorscheme("ayu")
check()
vim.cmd.colorscheme("habamax")
assert(vim.g.terminal_color_1 ~= expected[2], "Habamax must replace the ANSI palette")
vim.cmd.colorscheme("ayu")
check()
vim.cmd.colorscheme("habamax")
require("ayu").colorscheme()
check()
vim.cmd.colorscheme("habamax")
configure()
check()
vim.cmd.colorscheme("habamax")
vim.cmd.colorscheme("ayu")
check()
print("PASS Neovim terminal palette after native roundtrip, direct application and config reload")
