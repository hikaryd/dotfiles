return {
  'aserowy/tmux.nvim',
  config = function()
    -- Чтение регистров синхронное, как в upstream, но без отдельного shell
    -- на каждый буфер. Не кэшируем содержимое и не отключаем clipboard sync.
    -- Адаптер внутреннего wrapper нужно проверять при обновлении tmux.nvim.
    if vim.env.TMUX then
      local wrapper = require('tmux.wrapper.tmux')
      local socket = vim.split(vim.env.TMUX, ',', { plain = true })[1]
      local function read_tmux(args)
        local command = { 'tmux', '-S', socket }
        vim.list_extend(command, args)
        -- text=false сохраняет CRLF и завершающие переводы строк регистров.
        local result = vim.system(command, { text = false }):wait()
        if result.code ~= 0 then
          vim.notify(
            ('[tmux] %s (exit %d): %s'):format(args[1], result.code, result.stderr or ''),
            vim.log.levels.ERROR
          )
        end
        return result.stdout or ''
      end
      wrapper.get_buffer = function(name)
        return read_tmux({ 'show-buffer', '-b', name })
      end
      wrapper.get_buffer_names = function()
        return vim.split(read_tmux({ 'list-buffers', '-F', '#{buffer_name}' }), '\n', { trimempty = true })
      end
    end
    return require('tmux').setup()
  end,
}
