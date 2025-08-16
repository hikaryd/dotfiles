return {
  'voldikss/vim-floaterm',
  enabled = false,
  event = 'VeryLazy',
  cmd = {
    'FloatermNew',
    'FloatermToggle',
    'FloatermSend',
    'FloatermKill',
    'FloatermPrev',
    'FloatermNext',
  },
  init = function()
    vim.g.floaterm_width = 0.9
    vim.g.floaterm_height = 0.9
    vim.g.floaterm_wintitle = 0
    vim.g.floaterm_title = 'floaterm ($1/$2) [$3]'
    vim.g.floaterm_borderchars = '─│─│┌┐┘└'
    vim.g.floaterm_keymap_toggle = '<F12>'
  end,
  config = function()
    local M = {}

    --- @param name string
    --- @param cmd string|nil
    local function toggle_or_new(name, cmd)
      local bufnr = vim.fn['floaterm#terminal#get_bufnr'](name)
      if bufnr ~= -1 then
        vim.cmd('FloatermToggle ' .. name)
        return
      end

      local command = {
        'FloatermNew',
        '--name=' .. name,
        '--title=' .. name,
        '--wintype=float',
        '--autoclose=0',
      }

      if cmd and #cmd > 0 then
        table.insert(command, cmd)
      end

      vim.cmd(table.concat(command, ' '))
    end

    function M.toggle_main()
      toggle_or_new('main', 'nu')
    end

    function M.toggle_pytest_current()
      toggle_or_new('pytest', 'nu')
    end

    function M.toggle_claude()
      toggle_or_new('claude', 'source .venv/bin/activate && claude')
    end

    function M.toggle_lazygit()
      toggle_or_new('lazygit', 'lazygit')
    end

    package.loaded['float-term'] = M
  end,
}
