local M = {}

M.preserved_win_sizes = {}

function M.setup()
  vim.o.updatetime = 50

  local api, fn = vim.api, vim.fn

  local GENERAL = api.nvim_create_augroup('UserGeneral', { clear = true })
  local FLOATERM = api.nvim_create_augroup('UserFloaterm', { clear = true })
  local DIAG = api.nvim_create_augroup('UserDiagnosticsHoverSimple', { clear = true })

  -- Floaterm: в терминале ESC прячет окно
  api.nvim_create_autocmd('TermOpen', {
    group = FLOATERM,
    callback = function(ev)
      if vim.b[ev.buf].floaterm_name then
        vim.keymap.set('t', '<Esc>', [[<C-\><C-n>:FloatermToggle<CR>]], { buffer = ev.buf, silent = true, desc = 'Floaterm: прятать окно' })
        vim.keymap.set('t', '<C-t>', [[<C-\><C-n>:FloatermToggle<CR>]], { buffer = ev.buf, silent = true, desc = 'Floaterm: прятать окно' })
        vim.keymap.set('t', '<C-w>', [[<C-\><C-n>:FloatermToggle<CR>]], { buffer = ev.buf, silent = true, desc = 'Floaterm: прятать окно' })
        vim.keymap.set('t', '<C-l>', [[<C-\><C-n>:FloatermToggle<CR>]], { buffer = ev.buf, silent = true, desc = 'Floaterm: прятать окно' })
      end
    end,
  })

  -- Показываем диагностику по удержанию курсора
  api.nvim_create_autocmd('CursorHold', {
    group = DIAG,
    callback = function()
      local diag = vim.diagnostic.get(0, { lnum = fn.line '.' - 1 })
      if #diag == 0 then
        return
      end
      vim.diagnostic.open_float(nil, {
        scope = 'line',
        border = 'rounded',
        focusable = false,
        source = 'always',
        header = '',
        prefix = '',
      })
    end,
  })

  -- Подсветка yank'а
  api.nvim_create_autocmd('TextYankPost', {
    group = GENERAL,
    callback = function()
      vim.highlight.on_yank { higroup = 'IncSearch', timeout = 100 }
    end,
  })

  -- Сохраняем и восстанавливаем размеры окон после VimResized
  local function preserve_window_sizes()
    local sizes = {}
    for _, win in ipairs(api.nvim_list_wins()) do
      sizes[win] = {
        height = api.nvim_win_get_height(win),
        width = api.nvim_win_get_width(win),
      }
    end
    M.preserved_win_sizes = sizes
  end

  local function restore_window_sizes()
    if not M.preserved_win_sizes then
      return
    end
    for win, size in pairs(M.preserved_win_sizes) do
      if api.nvim_win_is_valid(win) and size then
        api.nvim_win_set_height(win, size.height)
        api.nvim_win_set_width(win, size.width)
      end
    end
  end

  api.nvim_create_autocmd('VimResized', {
    group = GENERAL,
    callback = function()
      preserve_window_sizes()
      vim.defer_fn(restore_window_sizes, 50)
    end,
  })

  -- Быстрые буферы, закрываемые на 'q'
  api.nvim_create_autocmd('FileType', {
    group = GENERAL,
    pattern = {
      'qf',
      'help',
      'man',
      'notify',
      'lspinfo',
      'spectre_panel',
      'startuptime',
      'tsplayground',
      'PlenaryTestPopup',
    },
    callback = function(ev)
      vim.bo[ev.buf].buflisted = false
      vim.keymap.set('n', 'q', '<cmd>close<CR>', { buffer = ev.buf, silent = true })
    end,
  })

  -- Markdown / gitcommit
  api.nvim_create_autocmd('FileType', {
    group = GENERAL,
    pattern = { 'gitcommit', 'markdown' },
    callback = function()
      vim.opt_local.wrap = true
      vim.opt_local.spell = true
    end,
  })
end

return M
