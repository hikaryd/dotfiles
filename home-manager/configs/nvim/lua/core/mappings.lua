local function map(mode, lhs, rhs, desc)
  vim.keymap.set(mode, lhs, rhs, { silent = true, desc = desc, noremap = false })
end

-- General
map('n', '<leader>q', '<cmd>quit<CR>', 'Quit Neovim')
map('n', 'q', '<cmd>quit<CR>', 'Quit Neovim')
map('n', ';', ':', 'Enter command mode')
map('n', '<C-s>', '<cmd>write<CR>', 'Save file')
map('n', '<C-c>', '<cmd>%y+<CR>', 'Copy whole file to system clipboard')
map('n', '<Esc>', '<cmd>nohlsearch<CR>', 'Clear search highlights')
map('n', 'v$', 'v$h', 'Visual: select to end of line minus one char')

-- File Explorer
-- map('n', '<Space>e', '<cmd>Yazi<CR>', 'Toggle file explorer')
-- map('n', '<Space>e', ':Triptych<CR>', 'Toggle file explorer')
map('n', '<Space>e', ':Neotree float reveal=true<CR>', 'Toggle file explorer')
-- map('n', '<Space>e', ':Fyler<CR>', 'Toggle file explorer')

-- Window Management
-- map('n', '<C-h>', '<cmd>SmartCursorMoveLeft<CR>', 'Resize window left')
-- map('n', '<C-j>', '<cmd>SmartCursorMoveDown<CR>', 'Resize window down')
-- map('n', '<C-k>', '<cmd>SmartCursorMoveUp<CR>', 'Resize window up')
-- map('n', '<C-l>', '<cmd>SmartCursorMoveRight<CR>', 'Resize window right')

map('n', '<leader>h', '<cmd>SmartCursorMoveLeft<CR>', 'Move to left split')
map('n', '<leader>j', '<cmd>SmartCursorMoveDown<CR>', 'Move to below split')
map('n', '<leader>k', '<cmd>SmartCursorMoveUp<CR>', 'Move to above split')
map('n', '<leader>l', '<cmd>SmartCursorMoveRight<CR>', 'Move to right split')
--
map('n', '<leader>v', '<cmd>vsplit<CR>', 'Vertical split window')

-- Moving lines
map('v', '<A-j>', "<cmd>m '>+1<CR>gv=gv", 'Visual: move lines down')
map('v', '<A-k>', "<cmd>m '<-2<CR>gv=gv", 'Visual: move lines up')
map('n', '<A-j>', '<cmd>m .+1<CR>==', 'Move current line down')
map('n', '<A-k>', '<cmd>m .-2<CR>==', 'Move current line up')

-- Buffer navigation
-- map('n', '<C-q>', '<cmd>bd!<CR>', 'Close current buffer')

map('n', '<leader>or', '<cmd>OverseerRun<CR>', 'Run overseer')

-- Float terminals
-- map('n', '<C-t>', '<cmd>lua require("float-term").toggle_main()<CR>', 'Toggle main terminal')
map('n', '<leader>p', '<cmd>lua require("float-term").toggle_pytest_current()<CR>', 'Pytest current file (floaterm)')
map('n', '<C-w>', '<cmd>lua require("float-term").toggle_claude()<CR>', 'Claude-code (floaterm)')
-- map('n', '<leader>g', '<cmd>lua require("float-term").toggle_lazygit()<CR>', 'Lazygit (floaterm)')
