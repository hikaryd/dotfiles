local function map(mode, lhs, rhs, desc)
  vim.keymap.set(mode, lhs, rhs, { silent = true, desc = desc, noremap = true })
end

-- General
map('n', '<leader>q', ':quit<Return>', 'Quit Neovim')
map('n', 'q', ':quit<Return>', 'Quit Neovim')
map('n', ';', ':', 'Enter command mode')
map('n', '<C-s>', ':write<CR>', 'Save file')
map('n', '<C-c>', '<cmd>%y+<CR>', 'Copy whole file to clipboard')
map('n', '<Esc>', '<cmd>noh<CR>', 'Clear search highlights')
map('n', 'v$', 'v$h')

-- File Explorer
map('n', '<space>e', ':Yazi<CR>')

-- DocString gen
map('n', '<space>dg', ':DocscribeGenerate<CR>')

-- Window Management
map('n', '<A-h>', ':SmartCursorResizeLeft<CR>')
map('n', '<A-j>', ':SmartCursorResizeDown<CR>')
-- moving between splits
map('n', '<leader>h', ':SmartCursorMoveLeft<CR>')
map('n', '<leader>l', ':SmartCursorMoveRight<CR>')
map('n', '<leader>j', ':SmartCursorMoveDown<CR>')
map('n', '<leader>k', ':SmartCursorMoveUp<CR>')

map('n', '<leader>v', ':vsplit<CR>', 'Vertical split')

map('v', '<A-j>', ":m '>+1<CR>gv=gv", 'Move selected lines down')
map('v', '<A-k>', ":m '<-2<CR>gv=gv", 'Move selected lines up')
map('n', '<A-k>', ':m .-2<CR>==', 'Move current line up')
map('n', '<A-j>', ':m .+1<CR>==', 'Move current line down')

for i = 1, 9 do
  map('n', '<C-' .. i .. '>', '<Cmd>BufferLineGoToBuffer ' .. i .. '<CR>')
end

map('n', '<leader>x', '<Cmd>BufferLinePickClose<CR>', 'Pick buffer to close')
map('n', '<C-q>', '<Cmd>bd!<CR>', 'Close current buffer')
