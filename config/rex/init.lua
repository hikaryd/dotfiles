local prefix = "ctrl+[KeyA]>"
local function bind(key, action, args)
  rex.bind(prefix .. key, action, args)
end

rex.action{
  name = "dots_close", title = "Close pane or window", category = "Dots",
  args = { type = "object", properties = { target = { type = "string", enum = { "pane", "window" } } }, required = { "target" }, additionalProperties = false },
  run = function(ctx, args)
    if type(args) ~= "table" or (args.target ~= "pane" and args.target ~= "window") then error("Unknown close target", 0) end
    if type(ctx) ~= "table" or type(ctx.session_id) ~= "string" or ctx.session_id == "" or type(ctx.block_id) ~= "string" or ctx.block_id == "" then
      error("Missing close context", 0)
    end
    local closed, err
    if args.target == "pane" then
      closed, err = rex.call("block.close", { session_id = ctx.session_id, block_id = ctx.block_id })
    else
      local result
      result, err = rex.session.describe_block({ session_id = ctx.session_id, block_id = ctx.block_id })
      if not result then error(err or "Cannot describe block", 0) end
      local block = type(result) == "table" and result.block
      if type(block) ~= "table" or block.block_id ~= ctx.block_id then error("Malformed block description", 0) end
      if type(block.window_id) ~= "string" or block.window_id == "" then error("Missing block window context", 0) end
      closed, err = rex.session.close_window({ session_id = ctx.session_id, window_id = block.window_id })
    end
    if not closed then error(err or "Close failed", 0) end
    return closed
  end,
}

bind("ctrl+[KeyA]", "pane.send_key", { key = "ctrl+a" })
bind("[KeyC]", "client.tab.new")
bind("[KeyB]", "client.tab.previous")
bind("[KeyN]", "client.tab.next")
bind("[Comma]", "client.tab.rename")
bind("[KeyW]", "dots_close", { target = "window" })
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
rex.bind("ctrl+[KeyW]", "dots_close", { target = "pane" })

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
local function session_id(id)
  if type(id) ~= "string" then return false end
  local uuid = id:match("^session:(%x%x%x%x%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%x%x%x%x%x%x%x%x)$")
  return uuid ~= nil
end
rex.action{
  name = "dots_last_session", title = "Last session", category = "Dots",
  args = { type = "object", additionalProperties = false },
  run = function(ctx)
    if type(ctx) ~= "table" or not session_id(ctx.session_id) or type(ctx.client_id) ~= "string" or ctx.client_id == "" then error("Missing last-session context", 0) end
    local result, err = rex.call("session.list", {})
    if type(result) ~= "table" or type(result.sessions) ~= "table" then error(err or "Cannot list sessions", 0) end
    local argv = { home .. "/.local/bin/dots-rex-last-session", "target", ctx.session_id }
    local live = {}
    for _, session in ipairs(result.sessions) do
      if type(session) ~= "table" or not session_id(session.session_id) then error("Malformed session list", 0) end
      live[session.session_id] = true
      argv[#argv + 1] = session.session_id
    end
    if not live[ctx.session_id] then error("Current session is closed", 0) end
    local output
    output, err = rex.exec(argv, { timeout = 3 })
    if not output or not output.ok or type(output.stdout) ~= "string" then error(err or (output and output.stderr) or "Last-session helper failed", 0) end
    local target = output.stdout:match("^%s*(.-)%s*$")
    if not session_id(target) or target == ctx.session_id or not live[target] then error("Invalid last-session target", 0) end
    rex.client.queue("session.select", { session_id = target })
  end,
}
bind("shift+[KeyL]", "dots_last_session")
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
      focus = false,
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
