return {
  {
    'okuuva/auto-save.nvim',
    enabled = false,
    cmd = 'ASToggle',
    event = { 'BufLeave', 'FocusLost' },
    opts = {
      enabled = true,
      trigger_events = {
        immediate_save = { 'BufLeave', 'FocusLost' },
        defer_save = { 'InsertLeave', 'TextChanged' },
        cancel_deferred_save = { 'InsertEnter' },
      },
      condition = nil,
      write_all_buffers = false,
      noautocmd = false,
      lockmarks = false,
      debounce_delay = 1000,
      debug = false,
    },
  },
}
