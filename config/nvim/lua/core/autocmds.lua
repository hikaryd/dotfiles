local M = {}

function M.setup()
  local api, fn = vim.api, vim.fn

  vim.o.updatetime = 100

  local GENERAL = api.nvim_create_augroup('UserGeneral', { clear = true })
  local DIAG = api.nvim_create_augroup('UserDiagnosticsHoverSimple', { clear = true })

  -- vim.api.nvim_create_autocmd({ 'BufLeave', 'WinLeave' }, {
  --   callback = function()
  --     for _, win in ipairs(vim.api.nvim_list_wins()) do
  --       local cfg = vim.api.nvim_win_get_config(win)
  --       if cfg and cfg.relative ~= '' then
  --         pcall(vim.api.nvim_win_close, win, true)
  --       end
  --     end
  --   end,
  -- })

  -- Подсветка yank'а
  api.nvim_create_autocmd('TextYankPost', {
    group = GENERAL,
    callback = function()
      vim.highlight.on_yank { higroup = 'IncSearch', timeout = 100 }
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
end

return M
