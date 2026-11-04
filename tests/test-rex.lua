local bindings, actions = {}, {}
local process, process_error, window_error, created
local description, description_error, write_result, write_error, writes, queued, queue_error
local closes, close_error, close_throws = {}, nil, nil
local ctx = { session_id = "session", block_id = "source" }
local function close(method, args)
  closes[#closes + 1] = { method = method, args = args }
  if close_throws then error(close_throws, 0) end
  if close_error then return nil, close_error end
  return { revision = 1 }
end
rex = {
  call = function(method, args)
    assert(method == "block.close")
    return close(method, args)
  end,
  bind = function(key, action, args)
    assert(not bindings[key], "duplicate binding: " .. key)
    bindings[key] = { action = action, args = args }
  end,
  action = function(action) actions[action.name] = action end,
  block = { call = function(creator, method, args)
    assert(creator == "com.superlogical.terminal")
    assert(args.block_id == "source")
    if method == "write" then
      assert(args.session_id == "session")
      writes[#writes + 1] = args.data
      if write_error then return nil, write_error end
      return write_result or { bytes_written = #args.data }
    end
    assert(method == "process")
    return process, process_error
  end },
  client = { queue = function(name, args)
    if queue_error then error(queue_error, 0) end
    queued = { action = name, args = args }
  end },
  layout = { block = function(layout) return layout end },
  session = { describe_block = function(args)
    assert(args.session_id == "session" and args.block_id == "source")
    return description, description_error
  end, close_window = function(args) return close("session.close_window", args) end,
  new_window = function(args)
    created = args
    if window_error then return nil, window_error end
    return { window_id = "new", block_ids = { "tool" } }
  end },
}
dofile("config/rex/init.lua")
local function native(key)
  local binding = assert(bindings[key], key)
  if binding.action ~= "dots_key" then return binding end
  writes, queued, closes = {}, nil, {}
  description = { block = { block_id = "source", window_id = "source-window", label = "ordinary", creator_name = "com.superlogical.terminal" } }
  actions.dots_key.run(ctx, binding.args)
  assert(#writes == 0)
  if key == "ctrl+[KeyW]" or key == "ctrl+[KeyA]>[KeyW]" then
    assert(not queued and #closes == 1)
    local call = closes[1]
    assert(call.args.session_id == ctx.session_id)
    local fields = 0
    for _ in pairs(call.args) do fields = fields + 1 end
    assert(fields == 2)
    if key == "ctrl+[KeyW]" then
      assert(call.method == "block.close" and call.args.block_id == ctx.block_id)
      return { action = "pane.close" }
    end
    assert(call.method == "session.close_window" and call.args.window_id == "source-window")
    return { action = "client.tab.close" }
  end
  assert(#closes == 0)
  return assert(queued)
end
assert(native("ctrl+[KeyA]>[KeyC]").action == "client.tab.new")
assert(native("ctrl+[KeyA]>ctrl+[KeyA]").args.key == "ctrl+a")
assert(native("ctrl+[KeyA]>shift+[Backslash]").args.direction == "right")
assert(native("ctrl+[Digit9]").args.index == 9)
assert(bindings["ctrl+[KeyO]"].action == "client.palette.toggle")
assert(bindings["ctrl+[KeyW]"], "Ctrl+W must be a direct pane-close binding")
assert(native("ctrl+[KeyW]").action == "pane.close")
assert(not bindings["ctrl+[KeyA]>ctrl+[KeyW]"])
for key, action in pairs({
  ["[KeyW]"] = "client.tab.close", ["[KeyC]"] = "client.tab.new",
  ["[KeyB]"] = "client.tab.previous", ["[KeyN]"] = "client.tab.next",
  ["[KeyU]"] = "session.switch", ["shift+[KeyN]"] = "session.new",
  ["[KeyM]"] = "pane.zoom", ["[KeyF]"] = "pane.zoom",
}) do
  assert(native("ctrl+[KeyA]>" .. key).action == action)
end
for key, direction in pairs({ H = "left", J = "down", K = "up", L = "right" }) do
  assert(native("ctrl+[Key" .. key .. "]").action == "pane.focus." .. direction)
  local resize = native("ctrl+shift+[Key" .. key .. "]")
  assert(resize.action == "pane.resize" and resize.args.direction == direction and resize.args.amount == 5)
end
for index = 1, 9 do
  assert(native("ctrl+[Digit" .. index .. "]").args.index == index)
  assert(native("ctrl+[KeyA]>[Digit" .. index .. "]").args.index == index)
end
local payloads = {
  ["ctrl+[KeyA]"] = "\1", ["[KeyC]"] = "c", ["[KeyB]"] = "b", ["[KeyN]"] = "n",
  ["[Comma]"] = ",", ["[KeyW]"] = "w", ["shift+[Backslash]"] = "|", ["[Minus]"] = "-",
  ["shift+[Minus]"] = "_", ["[KeyM]"] = "m", ["[KeyF]"] = "f", ["[Space]"] = " ",
  ["[Enter]"] = "\r", ["[KeyG]"] = "g", ["shift+[KeyY]"] = "Y",
}
local expected = { ["ctrl+[KeyW]"] = "\23", ["ctrl+shift+[ArrowLeft]"] = "\27[1;6D", ["ctrl+shift+[ArrowRight]"] = "\27[1;6C" }
for key, byte in pairs(payloads) do expected["ctrl+[KeyA]>" .. key] = "\1" .. byte end
for _, key in ipairs({ "H", "J", "K", "L" }) do
  expected["ctrl+[KeyA]>[Key" .. key .. "]"] = "\1" .. key:lower()
  expected["ctrl+[Key" .. key .. "]"] = string.char(key:byte() - 64)
  expected["ctrl+shift+[Key" .. key .. "]"] = "\1" .. key:lower()
end
for index = 1, 9 do
  expected["ctrl+[KeyA]>[Digit" .. index .. "]"] = "\1" .. index
  expected["ctrl+[Digit" .. index .. "]"] = "\1" .. index
end
for key, payload in pairs(expected) do
  local binding = assert(bindings[key], key)
  assert(binding.action == "dots_key" and binding.args.id == key)
  writes, queued, closes = {}, nil, {}
  description = { block = { block_id = "source", label = "dots:rmux-bridge", creator_name = "com.superlogical.terminal" } }
  actions.dots_key.run(ctx, binding.args)
  assert(#writes == 1 and writes[1] == payload and not queued and #closes == 0, key)
end
for key, binding in pairs(bindings) do
  if binding.action == "dots_key" then assert(expected[key], key) end
end
local function bridge_fails(context, id, message)
  writes, queued = {}, nil
  local ok, err = pcall(actions.dots_key.run, context, { id = id })
  assert(not ok and tostring(err):find(message, 1, true), tostring(err))
  assert(not queued)
end
local id = "ctrl+[KeyA]>[KeyC]"
bridge_fails({}, id, "context")
bridge_fails(ctx, "unknown", "Unknown")
description, description_error = nil, "describe failed"
bridge_fails(ctx, id, "describe failed")
description, description_error = {}, nil
bridge_fails(ctx, id, "Malformed")
description = { block = { block_id = "source", label = "dots:rmux-bridge", creator_name = "other" } }
bridge_fails(ctx, id, "Invalid bridge creator")
assert(#writes == 0)
description.block.label = "native"
queue_error = "queue failed"
bridge_fails(ctx, id, queue_error)
queue_error = nil
description.block.label = "dots:rmux-bridge"
description.block.creator_name = "com.superlogical.terminal"
write_error = "write failed"
bridge_fails(ctx, id, "write failed")
write_error, write_result = nil, { bytes_written = 1 }
bridge_fails(ctx, id, "Incomplete")
write_result = nil
for _, key in ipairs({ "ctrl+[KeyW]", "ctrl+[KeyA]>[KeyW]" }) do
  local function close_fails(context, message, attempts)
    closes = {}
    bridge_fails(context, key, message)
    assert(#writes == 0 and #closes == attempts)
  end
  close_fails({}, "context", 0)
  close_fails({ session_id = "session", block_id = "" }, "context", 0)
  description, description_error = nil, "describe failed"
  close_fails(ctx, "describe failed", 0)
  description, description_error = { block = { block_id = "other", label = "native", creator_name = "com.superlogical.terminal" } }, nil
  close_fails(ctx, "Malformed", 0)
  description.block.block_id = "source"
  if key == "ctrl+[KeyA]>[KeyW]" then
    close_fails(ctx, "Missing block window context", 0)
    for _, invalid in ipairs({ "", false, 42, {} }) do
      description.block.window_id = invalid
      close_fails(ctx, "Missing block window context", 0)
    end
  end
  description.block.window_id = "source-window"
  close_error = "close API failed"
  close_fails(ctx, close_error, 1)
  close_error, close_throws = nil, "close API threw"
  close_fails(ctx, close_throws, 1)
  close_throws = nil
end
local home = assert(os.getenv("HOME"))
process = { foreground = { cwd = "/repo with spaces/'$(literal)'" } }
for _, name in ipairs({ "git", "clip" }) do
  queued = nil
  assert(actions[name].run(ctx).window_id == "new")
  assert(queued.action == "session.select" and queued.args.session_id == "session" and queued.args.window_id == "new")
  assert(created.focus and created.layout.options.cwd == process.foreground.cwd)
  assert(created.layout.options.shell == "none")
  local argv = created.layout.options.command
  if name == "git" then assert(argv[1] == "lazygit") else
    assert(argv[1] == home .. "/.local/bin/dots-tools")
    assert(argv[2] == "desktop" and argv[3] == name)
  end
end
local function fails(message)
  created = nil
  local ok, err = pcall(actions.git.run, ctx)
  assert(not ok and tostring(err):find(message, 1, true))
end
created, queued = nil, nil
local ok, err = pcall(actions.git.run, { block_id = "source" })
assert(not ok and tostring(err):find("Missing tool session context", 1, true))
assert(not created and not queued)
process, process_error = nil, "source pane disappeared"
fails(process_error)
description = { block = { block_id = "source", label = "native", creator_name = "com.superlogical.terminal" } }
bridge_fails(ctx, "ctrl+[KeyA]>[KeyG]", process_error)
assert(not created)
process, process_error = {}, nil
fails("Cannot determine")
assert(not created)
process = { child = { cwd = "/fallback" } }
assert(actions.git.run(ctx))
assert(created.layout.options.cwd == "/fallback")
process.foreground = { pid = 42, user_id = 0, name = "sudo" }
assert(actions.git.run(ctx))
assert(created.layout.options.cwd == "/fallback")
window_error = "window creation failed"
fails(window_error)
bridge_fails(ctx, "ctrl+[KeyA]>[KeyG]", window_error)
print("PASS Rex bindings and actions")
