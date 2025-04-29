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
map('n', '<space>e', ':Triptych<CR>')

-- map('n', '<leader>tt', ':BufTermEnter<CR>')

-- Window Management
map('n', '<A-h>', require('smart-splits').resize_left)
map('n', '<A-j>', require('smart-splits').resize_down)
map('n', '<A-k>', require('smart-splits').resize_up)
map('n', '<A-l>', require('smart-splits').resize_right)
-- moving between splits
map('n', '<C-h>', require('smart-splits').move_cursor_left)
map('n', '<C-l>', require('smart-splits').move_cursor_right)
map('t', '<C-h>', require('smart-splits').move_cursor_left)
map('t', '<C-j>', require('smart-splits').move_cursor_down)
map('t', '<C-k>', require('smart-splits').move_cursor_up)
map('t', '<C-l>', require('smart-splits').move_cursor_right)
-- Split
map('n', '<C-b>', ':split<CR>', '')
map('n', '<C-v>', ':vsplit<CR>', '')

-- Line Movement
map('v', '<A-j>', ":m '>+1<CR>gv=gv", 'Move selected lines down')
map('v', '<A-k>', ":m '<-2<CR>gv=gv", 'Move selected lines up')
map('n', '<A-k>', ':m .-2<CR>==', 'Move current line up')
map('n', '<A-j>', ':m .+1<CR>==', 'Move current line down')
