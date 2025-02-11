-- Basic options that should be set first
vim.cmd 'set modifiable'

-- UI and Display
vim.opt.termguicolors = true
vim.opt.number = true
vim.opt.relativenumber = true
-- vim.opt.signcolumn = 'yes'
-- vim.opt.colorcolumn = '80'
vim.opt.pumblend = 0
vim.opt.winblend = 0
vim.opt.clipboard = 'unnamedplus'
vim.opt.laststatus = 3
vim.opt.equalalways = false

-- Cursor Styles
vim.opt.guicursor = {
  'n-v-c:block', -- Normal, visual, command-line: block cursor
  'i-ci-ve:ver25', -- Insert, command-line insert, visual-exclude: vertical bar cursor with 25% width
  'r-cr:hor20', -- Replace, command-line replace: horizontal bar cursor with 20% height
  'o:hor50', -- Operator-pending: horizontal bar cursor with 50% height
  'a:blinkwait700-blinkoff400-blinkon250', -- All modes: blinking settings
  'sm:block-blinkwait175-blinkoff150-blinkon175', -- Showmatch: block cursor with specific blinking settings
}

-- Editing Behavior
vim.opt.mouse = 'n'
vim.opt.breakindent = true
vim.opt.undofile = true
vim.opt.ignorecase = true
vim.opt.smartcase = true
vim.opt.hidden = true
vim.opt.splitbelow = true
vim.opt.splitright = true

-- Search Enhancements
vim.opt.showmatch = true
vim.opt.incsearch = true
vim.opt.hlsearch = true
vim.opt.inccommand = 'nosplit'

-- Display Improvements
vim.opt.scrolloff = 31
vim.opt.sidescrolloff = 31
vim.opt.wrap = false
vim.opt.fileencoding = 'utf-8'
vim.opt.spelllang = 'en,ru'

vim.o.completeopt = 'menu,menuone'
