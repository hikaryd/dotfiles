return {
  'folke/snacks.nvim',
  priority = 10000,
  lazy = false,
  opts = {
    input = { enabled = true },
    bigfile = { enabled = true },
    notifier = {
      enabled = true,
      timeout = 3000,
    },
    quickfile = { enabled = true },
    statuscolumn = { enabled = true },
    words = { enabled = true },
    styles = {
      notification = {
        wo = { wrap = true },
      },
    },
    lazygit = {
      enabled = true,
    },
    animate = {
      enabled = true,
    },
    indent = {
      enabled = true,
    },
    toggle = {
      enabled = true,
    },
    image = { enabled = true },
    picker = {
      sources = {
        explorer = {
          auto_close = true,
        },
      },
    },
    {
      debounce = 200, -- time in ms to wait before updating
      notify_jump = false, -- show a notification when jumping
      notify_end = true, -- show a notification when reaching the end
      foldopen = true, -- open folds after jumping
      jumplist = true, -- set jump point before jumping
      modes = { 'n', 'i', 'c' }, -- modes to show references
    },
    {
      left = { 'mark', 'sign' }, -- priority of signs on the left (high to low)
      right = { 'fold', 'git' }, -- priority of signs on the right (high to low)
      folds = {
        open = true, -- show open fold icons
        git_hl = true, -- use Git Signs hl for fold icons
      },
      git = {
        patterns = { 'GitSign', 'MiniDiffSign' },
      },
      refresh = 50,
    },
    dashboard = {
      enabled = true,
      config = {
        header = {
          '                                                     ',
          '  ███╗   ██╗███████╗ ██████╗ ██╗   ██╗██╗███╗   ███╗',
          '  ████╗  ██║██╔════╝██╔═══██╗██║   ██║██║████╗ ████║',
          '  ██╔██╗ ██║█████╗  ██║   ██║██║   ██║██║██╔████╔██║',
          '  ██║╚██╗██║██╔══╝  ██║   ██║╚██╗ ██╔╝██║██║╚██╔╝██║',
          '  ██║ ╚████║███████╗╚██████╔╝ ╚████╔╝ ██║██║ ╚═╝ ██║',
          '  ╚═╝  ╚═══╝╚══════╝ ╚═════╝   ╚═══╝  ╚═╝╚═╝     ╚═╝',
          '                                                     ',
        },
        shortcuts = {
          { desc = '󰊳 Update', group = '@property', key = 'u', action = 'Lazy update' },
          { desc = ' Files', group = '@property', key = 'f', action = 'Telescope find_files' },
          { desc = ' Apps', group = '@property', key = 'a', action = 'Telescope app' },
          { desc = ' dotfiles', group = '@property', key = 'd', action = 'Telescope dotfiles' },
        },
        packages = { enabled = true },
        sections = {
          { section = 'header' },
          {
            pane = 2,
            section = 'terminal',
            cmd = 'colorscript -e square',
            height = 5,
            padding = 1,
          },
          { section = 'keys', gap = 1, padding = 1 },
          { pane = 2, icon = ' ', title = 'Recent Files', section = 'recent_files', indent = 2, padding = 1 },
          { pane = 2, icon = ' ', title = 'Projects', section = 'projects', indent = 2, padding = 1 },
          {
            pane = 2,
            icon = ' ',
            title = 'Git Status',
            section = 'terminal',
            enabled = function()
              return Snacks.git.get_root() ~= nil
            end,
            cmd = 'git status --short --branch --snacrenames',
            height = 5,
            padding = 1,
            ttl = 5 * 60,
            indent = 3,
          },
          { section = 'startup' },
        },
        footer = function()
          local stats = require('lazy').stats()
          return { '⚡ Neovim loaded ' .. stats.loaded .. '/' .. stats.count .. ' plugins' }
        end,
      },
    },
  },
  keys = {
    {
      '<leader>gg',
      function()
        Snacks.lazygit()
      end,
      desc = 'Lazygit',
    },
    {
      '<leader>gf',
      function()
        Snacks.lazygit.log_file()
      end,
      desc = 'Lazygit Current File History',
    },
    {
      '<leader>n',
      function()
        Snacks.notifier.show_history()
      end,
      desc = 'Notification History',
    },
    {
      '<leader>/',
      function()
        Snacks.picker.grep()
      end,
      desc = 'Grep',
    },
    {
      '<leader><space>',
      function()
        Snacks.picker.files {
          finder = 'files',
          format = 'file',
          show_empty = true,
          supports_live = true,
        }
      end,
      desc = 'Find Files',
    },
    {
      '<leader>fb',
      function()
        Snacks.picker.buffers()
      end,
      desc = 'Buffers',
    },
    {
      '<leader>fc',
      function()
        Snacks.picker.files { cwd = vim.fn.stdpath 'config' }
      end,
      desc = 'Find Config File',
    },
    {
      '<leader>fo',
      function()
        Snacks.picker.recent()
      end,
      desc = 'Recent',
    },
    {
      '<leader>sb',
      function()
        Snacks.picker.lines()
      end,
      desc = 'Buffer Lines',
    },
    {
      '<leader>xd',
      function()
        Snacks.picker.diagnostics()
      end,
      desc = 'Diagnostics',
    },
    -- {
    --   '<leader>e',
    --   function()
    --     Snacks.explorer.open()
    --   end,
    --   desc = 'Diagnostics',
    -- },
    {
      '<leader>qp',
      function()
        Snacks.picker.projects()
      end,
      desc = 'Projects',
    },
    -- {
    --   'gd',
    --   function()
    --     Snacks.picker.lsp_definitions()
    --   end,
    --   desc = 'Goto Definition',
    -- },
    {
      'gr',
      function()
        Snacks.picker.lsp_references()
      end,
      nowait = true,
      desc = 'References',
    },
  },
  init = function()
    vim.api.nvim_create_autocmd('User', {
      pattern = 'VeryLazy',
      callback = function()
        Snacks.toggle.option('spell', { name = 'Spelling' }):map '<leader>us'
      end,
    })
  end,
}
