return {
  'aserowy/tmux.nvim',
  cond = function() return vim.env.RMUX ~= nil or vim.env.HERDR_ENV ~= '1' end,
  config = function()
    -- Чтение регистров синхронное, как в upstream, но без отдельного shell
    -- на каждый буфер. Не кэшируем содержимое и не отключаем clipboard sync.
    -- Адаптер внутреннего wrapper нужно проверять при обновлении tmux.nvim.
    if vim.env.RMUX or vim.env.TMUX then
      local wrapper = require('tmux.wrapper.tmux')
      local identity = vim.env.RMUX or vim.env.TMUX
      local socket = identity:match('^(.+),%d+,%d+$') or identity
      local binary = vim.env.RMUX and 'rmux' or 'tmux'
      local function execute(args, stdin, text)
        local command = { binary, '-S', socket }
        vim.list_extend(command, args)
        -- text=false сохраняет CRLF и завершающие переводы строк регистров.
        local result = vim.system(command, { text = text, stdin = stdin }):wait()
        if result.code ~= 0 then
          vim.notify(
            ('[%s] %s (exit %d): %s'):format(binary, args[1], result.code, result.stderr or ''),
            vim.log.levels.ERROR
          )
        end
        return result.stdout or ''
      end
      -- All navigation, resize and clipboard writes use the native backend.
      -- Keep upstream's synchronous contract and forward stdin without a shell.
      if vim.env.RMUX then
        wrapper.execute = function(args, stdin) return execute(args, stdin, true) end
      end
      wrapper.get_buffer = function(name)
        return execute({ 'show-buffer', '-b', name }, nil, false)
      end
      wrapper.get_buffer_names = function()
        return vim.split(execute({ 'list-buffers', '-F', '#{buffer_name}' }, nil, false), '\n', { trimempty = true })
      end
    end
    return require('tmux').setup()
  end,
}
