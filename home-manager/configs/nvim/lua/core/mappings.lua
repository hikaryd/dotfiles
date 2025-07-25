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
map('n', '<Space>e', ':Fyler kind=split:left<CR>', 'Toggle file explorer')

-- Window Management
map('n', '<A-h>', '<cmd>SmartCursorMoveLeft<CR>', 'Resize window left')
map('n', '<A-j>', '<cmd>SmartCursorMoveDown<CR>', 'Resize window down')
map('n', '<A-k>', '<cmd>SmartCursorMoveUp<CR>', 'Resize window up')
map('n', '<A-l>', '<cmd>SmartCursorMoveRight<CR>', 'Resize window right')

-- map('n', '<leader>h', '<cmd>SmartCursorMoveLeft<CR>', 'Move to left split')
-- map('n', '<leader>j', '<cmd>SmartCursorMoveDown<CR>', 'Move to below split')
-- map('n', '<leader>k', '<cmd>SmartCursorMoveUp<CR>', 'Move to above split')
-- map('n', '<leader>l', '<cmd>SmartCursorMoveRight<CR>', 'Move to right split')
--
map('n', '<leader>v', '<cmd>vsplit<CR>', 'Vertical split window')

-- Moving lines
map('v', '<A-j>', "<cmd>m '>+1<CR>gv=gv", 'Visual: move lines down')
map('v', '<A-k>', "<cmd>m '<-2<CR>gv=gv", 'Visual: move lines up')
map('n', '<A-j>', '<cmd>m .+1<CR>==', 'Move current line down')
map('n', '<A-k>', '<cmd>m .-2<CR>==', 'Move current line up')

-- Buffer navigation
-- for i = 1, 9 do
--   map('n', '<C-' .. i .. '>', '<cmd>BufferLineGoToBuffer ' .. i .. '<CR>', 'Go to buffer ' .. i)
-- end
map('n', '<C-q>', '<cmd>bd!<CR>', 'Close current buffer')

-- Docstring rewrite (PrtRewrite)
map(
  'v',
  '<leader>pd',
  [[<cmd>PrtRewrite Сгенерируй докстринг для функции. Описание пиши на русском. Не забывай про кавычки. Вот пример докстринга: def func(arg1, arg2): Тут описание :param arg1: Описание arg1. :param arg2: Описание arg2. :raise: ValueError if arg1 больше arg2 :return: Описание того, что возвращается<CR>]],
  'Generate docstring for function'
)

map('n', '<leader>or', '<cmd>OverseerRun<CR>', 'Run overseer')

-- Float terminals
map('n', '<leader>tt', '<cmd>lua require("float-term").toggle_main()<CR>', 'Toggle main terminal')
map('n', '<leader>tp', '<cmd>lua require("float-term").toggle_pytest_current()<CR>', 'Pytest current file (floaterm)')
map('n', '<leader>tl', '<cmd>lua require("float-term").toggle_pytest_last_failed()<CR>', 'Pytest last failed (floaterm)')
map('n', '<leader>tc', '<cmd>lua require("float-term").toggle_claude()<CR>', 'Claude-code (floaterm)')
map('n', '<leader>tg', '<cmd>lua require("float-term").toggle_lazygit()<CR>', 'Lazygit (floaterm)')
map('n', '<leader>ts', function()
  local file = vim.fn.fnameescape(vim.api.nvim_buf_get_name(0))
  vim.cmd('FloatermSend pytest -q ' .. file)
end, 'Send pytest current file to pytest terminal')

-- LSP
-- map('n', 'gd', '<cmd>lua vim.lsp.buf.definition()<CR>', 'LSP: go to definition')
-- map('n', 'gr', '<cmd>lua vim.lsp.buf.references()<CR>', 'LSP: find references')
-- map('n', 'K', '<cmd>lua vim.lsp.buf.hover()<CR>', 'LSP: hover documentation')
-- map('n', '<leader>ca', '<cmd>lua vim.lsp.buf.code_action()<CR>', 'LSP: code action')
-- map('v', '<leader>ca', '<cmd>lua vim.lsp.buf.range_code_action()<CR>', 'LSP: code action on selection')
-- map('n', 'rn', '<cmd>lua vim.lsp.buf.rename()<CR>', 'LSP: rename symbol')
-- map('n', 'gj', '<cmd>lua vim.diagnostic.goto_next()<CR>', 'LSP: next diagnostic')
-- map('n', 'gk', '<cmd>lua vim.diagnostic.goto_prev()<CR>', 'LSP: previous diagnostic')
-- map('n', '<leader>o', '<cmd>lua vim.lsp.buf.document_symbol()<CR>', 'LSP: document symbols')
-- map('i', '<C-k>', '<cmd>lua vim.lsp.buf.signature_help()<CR>', 'LSP: signature help')
-- map('n', '<leader>sh', '<cmd>lua vim.lsp.buf.signature_help()<CR>', 'LSP: signature help')
