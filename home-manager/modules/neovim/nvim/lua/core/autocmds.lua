local M = {}

M.preserved_win_sizes = {}

function M.setup()
  local autocmd = vim.api.nvim_create_autocmd
  local augroup = vim.api.nvim_create_augroup

  local general = augroup('General Settings', { clear = true })

  autocmd('TextYankPost', {
    group = general,
    callback = function()
      vim.highlight.on_yank { higroup = 'IncSearch', timeout = 100 }
    end,
  })

  local function preserve_window_sizes()
    local win_sizes = {}
    for _, win in ipairs(vim.api.nvim_list_wins()) do
      win_sizes[win] = {
        height = vim.api.nvim_win_get_height(win),
        width = vim.api.nvim_win_get_width(win),
      }
    end
    M.preserved_win_sizes = win_sizes
  end

  local function restore_window_sizes()
    if not M.preserved_win_sizes then
      return
    end
    for _, win in ipairs(vim.api.nvim_list_wins()) do
      local size = M.preserved_win_sizes[win]
      if size then
        vim.api.nvim_win_set_height(win, size.height)
        vim.api.nvim_win_set_width(win, size.width)
      end
    end
  end

  autocmd('VimResized', {
    group = general,
    callback = function()
      preserve_window_sizes()
      vim.defer_fn(function()
        restore_window_sizes()
      end, 50)
    end,
  })

  autocmd('FileType', {
    group = general,
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
    callback = function(event)
      vim.bo[event.buf].buflisted = false
      vim.keymap.set('n', 'q', '<cmd>close<cr>', { buffer = event.buf, silent = true })
    end,
  })

  autocmd('FileType', {
    group = general,
    pattern = { 'gitcommit', 'markdown' },
    callback = function()
      vim.opt_local.wrap = true
      vim.opt_local.spell = true
    end,
  })
end

return M
