-- Совместимость blink-emoji со старым API задач после перехода Blink на blink.lib.
local task = require('blink.lib.task')

return {
  task = {
    empty = function()
      return task.resolve(nil)
    end,
  },
}
