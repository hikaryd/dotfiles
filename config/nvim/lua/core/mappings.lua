local function map(mode, lhs, rhs, desc)
	vim.keymap.set(mode, lhs, rhs, { silent = true, desc = desc, noremap = true })
end

-- General
map("n", "<leader>q", "<cmd>quit<CR>", "Quit Neovim")
map("n", "q", "<cmd>quit<CR>", "Quit Neovim")
map("n", ";", ":", "Enter command mode")
map("n", "<C-s>", "<cmd>write<CR>", "Save file")
map("n", "<C-c>", "<cmd>%y+<CR>", "Copy whole file to system clipboard")
map("n", "<Esc>", "<cmd>nohlsearch<CR>", "Clear search highlights")
map("n", "v$", "v$h", "Visual: select to end of line minus one char")

-- File Explorer
-- map('n', '<Space>e', ':Neotree float reveal=true<CR>', 'Toggle file explorer')
local fyler = require("fyler")
vim.keymap.set("n", "<leader>e", function()
	fyler.open({ kind = "split_left_most" })
end, { desc = "Open Fyler View" })

map("n", "<leader>h", '<cmd>lua require("tmux").move_left()<CR>', "Move to left split")
map("n", "<leader>j", '<cmd>lua require("tmux").move_bottom()<CR>', "Move to below split")
map("n", "<leader>k", '<cmd>lua require("tmux").move_top()<CR>', "Move to above split")
map("n", "<leader>l", '<cmd>lua require("tmux").move_right()<CR>', "Move to right split")
--
map("n", "<leader>v", "<cmd>vsplit<CR>", "Vertical split window")

-- Outline
map("n", "<leader>o", "<cmd>Outline<CR>", "Toggle outline")

-- Moving lines
map("v", "<A-j>", "<cmd>m '>+1<CR>gv=gv", "Visual: move lines down")
map("v", "<A-k>", "<cmd>m '<-2<CR>gv=gv", "Visual: move lines up")
map("n", "<A-j>", "<cmd>m .+1<CR>==", "Move current line down")
map("n", "<A-k>", "<cmd>m .-2<CR>==", "Move current line up")

map("n", "<leader>or", "<cmd>OverseerRun<CR>", "Run overseer")

map("n", "K", "<cmd>Lspsaga hover_doc<CR>", "Saga Hover")
map("n", "<leader>do", "<cmd>Lspsaga show_cursor_diagnostics<CR>", "Saga diag (focuses)")
