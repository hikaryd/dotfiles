local prefix = "ctrl+[KeyA]>"
local routes = {}
local tools = {}
local bridge_keys = {
  ["ctrl+[KeyA]"] = "\1", ["[KeyC]"] = "c", ["[KeyB]"] = "b", ["[KeyN]"] = "n",
  ["[Comma]"] = ",", ["[KeyW]"] = "w", ["shift+[Backslash]"] = "|",
  ["[Minus]"] = "-", ["shift+[Minus]"] = "_", ["[KeyM]"] = "m", ["[KeyF]"] = "f",
  ["[Space]"] = " ", ["[Enter]"] = "\r", ["[KeyG]"] = "g", ["shift+[KeyY]"] = "Y",
}
local function route(key, action, args, data)
  routes[key] = { action = action, args = args, data = data }
  rex.bind(key, "dots_key", { id = key })
end
local function bind(key, action, args)
  local byte = bridge_keys[key]
  if byte then route(prefix .. key, action, args, "\1" .. byte)
  else rex.bind(prefix .. key, action, args) end
end

rex.action{
  name = "dots_key", title = "Dots key", category = "Dots",
  args = { type = "object", properties = { id = { type = "string" } }, required = { "id" }, additionalProperties = false },
  run = function(ctx, args)
    local entry = type(args) == "table" and routes[args.id]
    if not entry then error("Unknown dots key", 0) end
    if type(ctx) ~= "table" or type(ctx.session_id) ~= "string" or ctx.session_id == "" or type(ctx.block_id) ~= "string" or ctx.block_id == "" then
      error("Missing dots key context", 0)
    end
    local result, err = rex.session.describe_block({ session_id = ctx.session_id, block_id = ctx.block_id })
    if not result then error(err or "Cannot describe block", 0) end
    local block = type(result) == "table" and result.block
    if type(block) ~= "table" or type(block.label) ~= "string" or type(block.creator_name) ~= "string" or block.block_id ~= ctx.block_id then
      error("Malformed block description", 0)
    end
    -- Temporary marker for legacy rmux wrappers until those sessions end.
    if block.label == "dots:rmux-bridge" then
      if block.creator_name ~= "com.superlogical.terminal" then error("Invalid bridge creator", 0) end
      local written
      written, err = rex.block.call("com.superlogical.terminal", "write", { session_id = ctx.session_id, block_id = ctx.block_id, data = entry.data })
      if not written then error(err or "Terminal write failed", 0) end
      if type(written) ~= "table" or written.bytes_written ~= #entry.data then error("Incomplete terminal write", 0) end
      return written
    end
    if entry.action == "pane.close" or entry.action == "client.tab.close" then
      local closed
      if entry.action == "pane.close" then
        closed, err = rex.call("block.close", { session_id = ctx.session_id, block_id = ctx.block_id })
      else
        if type(block.window_id) ~= "string" or block.window_id == "" then error("Missing block window context", 0) end
        closed, err = rex.session.close_window({ session_id = ctx.session_id, window_id = block.window_id })
      end
      if not closed then error(err or "Close failed", 0) end
      return closed
    end
    if tools[entry.action] then return tools[entry.action](ctx) end
    rex.client.queue(entry.action, entry.args)
    return true
  end,
}

bind("ctrl+[KeyA]", "pane.send_key", { key = "ctrl+a" })
bind("[KeyC]", "client.tab.new")
bind("[KeyB]", "client.tab.previous")
bind("[KeyN]", "client.tab.next")
bind("[Comma]", "client.tab.rename")
bind("[KeyW]", "client.tab.close")
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
route("ctrl+[KeyW]", "pane.close", nil, "\23")

for key, direction in pairs({ H = "left", J = "down", K = "up", L = "right" }) do
  bridge_keys["[Key" .. key .. "]"] = key:lower()
  bind("[Key" .. key .. "]", "pane.resize", { direction = direction, amount = 5 })
  route("ctrl+[Key" .. key .. "]", "pane.focus." .. direction, nil, string.char(key:byte() - 64))
  route("ctrl+shift+[Key" .. key .. "]", "pane.resize", { direction = direction, amount = 5 }, "\1" .. key:lower())
end
for index = 1, 9 do
  bridge_keys["[Digit" .. index .. "]"] = tostring(index)
  bind("[Digit" .. index .. "]", "client.tab.goto", { index = index })
  route("ctrl+[Digit" .. index .. "]", "client.tab.goto", { index = index }, "\1" .. index)
end
route("ctrl+shift+[ArrowLeft]", "client.tab.move", { direction = "backward" }, "\27[1;6D")
route("ctrl+shift+[ArrowRight]", "client.tab.move", { direction = "forward" }, "\27[1;6C")

local home = assert(os.getenv("HOME"))
local function tool(name, title, key, command)
  local function run(ctx)
    if type(ctx) ~= "table" or type(ctx.session_id) ~= "string" or ctx.session_id == "" then
      error("Missing tool session context", 0)
    end
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
    rex.client.queue("session.select", { session_id = ctx.session_id, window_id = window.window_id })
    return window
  end
  tools[name] = run
  rex.action{
    name = name,
    title = title,
    category = "Dots",
    args = { type = "object", additionalProperties = false },
    run = run,
  }
  bind(key, name)
end

tool("git", "Lazygit", "[KeyG]", { "lazygit" })
tool("clip", "Clipboard", "shift+[KeyY]", { home .. "/.local/bin/dots-tools", "desktop", "clip" })
