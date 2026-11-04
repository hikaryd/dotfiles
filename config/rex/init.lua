local prefix = "ctrl+[KeyA]>"
local function bind(key, action, args)
  rex.bind(prefix .. key, action, args)
end

bind("ctrl+[KeyA]", "pane.send_key", { key = "ctrl+a" })
bind("[KeyC]", "client.tab.new")
bind("[KeyB]", "client.tab.previous")
bind("[KeyN]", "client.tab.next")
bind("[Comma]", "client.tab.rename")
bind("[KeyW]", "client.tab.close")
bind("ctrl+[KeyW]", "pane.close")
bind("shift+[KeyN]", "session.new")
bind("shift+[KeyW]", "session.rename")
bind("[KeyU]", "session.switch")
bind("shift+[KeyT]", "session.switch")
bind("[BracketLeft]", "session.previous")
bind("[BracketRight]", "session.next")
bind("shift+[Backslash]", "pane.split", { direction = "right" })
bind("[Minus]", "pane.split", { direction = "down" })
bind("shift+[Minus]", "pane.split", { direction = "down" })
bind("[KeyM]", "pane.zoom")
bind("[KeyF]", "pane.zoom")
bind("[Space]", "pane.focus_previous")
bind("[Enter]", "pane.focus_previous")
bind("shift+[KeyO]", "client.palette.toggle")
bind("shift+[KeyP]", "pane.go_to_directory")
bind("shift+[KeyF]", "client.find.open")
bind("shift+[KeyB]", "client.sidebar.activity.toggle")
bind("shift+[KeyV]", "client.sidebar.activity.toggle")
rex.bind("ctrl+[KeyO]", "client.palette.toggle")

for key, direction in pairs({ H = "left", J = "down", K = "up", L = "right" }) do
  bind("[Key" .. key .. "]", "pane.resize", { direction = direction, amount = 5 })
  rex.bind("ctrl+[Key" .. key .. "]", "pane.focus." .. direction)
  rex.bind("ctrl+shift+[Key" .. key .. "]", "pane.resize", { direction = direction, amount = 5 })
end
for index = 1, 9 do
  bind("[Digit" .. index .. "]", "client.tab.goto", { index = index })
  rex.bind("ctrl+[Digit" .. index .. "]", "client.tab.goto", { index = index })
end
rex.bind("ctrl+shift+[ArrowLeft]", "client.tab.move", { direction = "backward" })
rex.bind("ctrl+shift+[ArrowRight]", "client.tab.move", { direction = "forward" })

local home = assert(os.getenv("HOME"))
local function tool(name, title, key, command)
  rex.action{
    name = name,
    title = title,
    category = "Dots",
    args = { type = "object", additionalProperties = false },
    run = function(ctx)
      local process, err = rex.block.call("com.superlogical.terminal", "process", { block_id = ctx.block_id })
      if not process then error(err, 0) end
      local cwd = (process.foreground or {}).cwd or (process.child or {}).cwd
      if not cwd then error("Cannot determine the current pane's directory", 0) end
      local window
      window, err = rex.session.new_window{
        window_label = title,
        focus = true,
        layout = rex.layout.block{
          flavor = "com.superlogical.terminal.shell",
          options = {
            shell = "none", command = command, cwd = cwd,
            path_prepend = { home .. "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin" },
          },
        },
      }
      if not window then error(err, 0) end
      return window
    end,
  }
  bind(key, name)
end

tool("git", "Lazygit", "[KeyG]", { "lazygit" })
tool("clip", "Clipboard", "shift+[KeyY]", { home .. "/.local/bin/dots-tools", "desktop", "clip" })
