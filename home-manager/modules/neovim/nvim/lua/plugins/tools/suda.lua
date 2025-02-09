return {
  'lambdalisue/vim-suda',
  event = 'VeryLazy',
  config = function()
    vim.g.suda_smart_edit = 1
    vim.cmd [[cab ss SudaWrite]]
  end,
}
