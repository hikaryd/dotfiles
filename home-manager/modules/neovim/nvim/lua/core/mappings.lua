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
-- map('n', '<space>e', ':Oil<CR>')

map('n', '<leader>b', ':DBUIToggle<CR>')

--Gitlab
map('n', '<leader>gcm', ':lua require("gitlab").choose_merge_request()<CR>')

-- Window Management
map('n', '<C-h>', '<C-w>h', 'Move to left window')
map('n', '<C-k>', '<C-w>k', 'Move to upper window')
map('n', '<C-j>', '<C-w>j', 'Move to lower window')
map('n', '<C-l>', '<C-w>l', 'Move to right window')
map('n', '<leader>sv', '<cmd>:vsplit<CR>', 'Vertical split window')

-- Line Movement
map('v', '<A-j>', ":m '>+1<CR>gv=gv", 'Move selected lines down')
map('v', '<A-k>', ":m '<-2<CR>gv=gv", 'Move selected lines up')
map('n', '<A-k>', ':m .-2<CR>==', 'Move current line up')
map('n', '<A-j>', ':m .+1<CR>==', 'Move current line down')

-- Aerial
map('n', '<leader>aet', '<cmd>AerialToggle!<CR>', 'Toggle Aerial')
