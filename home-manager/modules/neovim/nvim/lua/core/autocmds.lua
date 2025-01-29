local M = {}

function M.setup()
  local autocmd = vim.api.nvim_create_autocmd
  local augroup = vim.api.nvim_create_augroup

  local general = augroup('General Settings', { clear = true })

  -- Highlight on yank
  autocmd('TextYankPost', {
    group = general,
    callback = function()
      vim.highlight.on_yank { higroup = 'IncSearch', timeout = 100 }
    end,
  })

  -- Resize splits if window got resized
  autocmd({ 'VimResized' }, {
    group = general,
    callback = function()
      vim.cmd 'tabdo wincmd ='
    end,
  })

  -- Close some filetypes with <q>
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

  -- Set wrap and spell in markdown and gitcommit
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
