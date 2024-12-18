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
-- map('n', '<leader>e', '<cmd>Neotree toggle<CR>', 'Toggle Neo-tree')
map('n', '<space>e', ':Telescope file_browser path=%:p:h select_buffer=true<CR>')

map('n', '<leader>b', ':DBUIToggle<CR>')

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
map('n', '<leader>a', '<cmd>AerialToggle!<CR>', 'Toggle Aerial')

-- Telescope
map('n', '<leader>fw', '<cmd>Telescope live_grep<CR>', 'Search for text in files')
map('n', '<leader>fo', '<cmd>Telescope oldfiles<CR>', 'Open recent files')
map('n', '<leader>ff', '<cmd>Telescope find_files<CR>', 'Open recent files')

-- Parrot (AI)
map('n', '<leader>pc', ':PrtChatToggle<CR>', 'Toggle current chat')
map('n', '<leader>pf', ':PrtChatFinder<CR>', 'Open chat search')
map(
  'v',
  '<leader>pd',
  ':PrtRewrite Сгенерируй докстринг для функции. Описание пиши на русском. Не забывай про кавычки. Вот пример докстринга: def func(arg1, arg2): Тут описание :param arg1: Описание arg1. :type arg1: int :param arg2: Описание arg2. :type arg2: int :raise: ValueError if arg1 is больше to arg2 :return: Описание того, что возвращается :rtype: bool <CR>',
  'Generate docstring for function'
)
map(
  'v',
  '<leader>pl',
  ":PrtRewrite Добавь логи в этот код, используя logger. Не изменяй существующий код, только добавь логи. Не добавляй импорты или другие изменения. Логи должны быть информативными и помогать в отладке. Используй разные уровни логирования (debug, info, warning, error) в зависимости от контекста. Вот пример того, как нужно добавлять логи: logger.debug('Начало выполнения функции example_function') logger.info(f'Получено значение: {value}') logger.warning('Предупреждение: достигнут лимит попыток') logger.error(f'Ошибка при обработке данных: {str(e)}') <CR>",
  'Add logs to selected code'
)
map('v', '<leader>pf', ':PrtRewrite Исправь ошибку<CR>', 'Fix error in selected code')
map('v', '<leader>pt', ':PrtRewrite Сгенерируй юнит-тесты (pytest)<CR>', 'Generate unit tests for selected code')
map(
  'v',
  '<leader>po',
  ':PrtRewrite Оптимизируй этот код для лучшей производительности<CR>',
  'Optimize selected code for better performance'
)
