local bindings, actions = {}, {}
local process, process_error, window_error, created, queued
local description, description_error, description_throws, describes
local closes, close_error, close_throws = {}, nil, nil
local ctx = { session_id = "session", block_id = "source" }
local function close(method, args)
  closes[#closes + 1] = { method = method, args = args }
  if close_throws then error(close_throws, 0) end
  if close_error then return nil, close_error end
  return { revision = 1 }
end
rex = {
  call = function(method, args) assert(method == "block.close") return close(method, args) end,
  bind = function(key, action, args)
    assert(not bindings[key], "duplicate binding: " .. key)
    bindings[key] = { action = action, args = args }
  end,
  action = function(action) assert(not actions[action.name]) actions[action.name] = action end,
  block = { call = function(creator, method, args)
    assert(creator == "com.superlogical.terminal" and method == "process" and args.block_id == "source")
    return process, process_error
  end },
  client = { queue = function(name, args) queued = { action = name, args = args } end },
  layout = { block = function(layout) return layout end },
  session = { describe_block = function(args)
    describes = describes + 1
    assert(args.session_id == "session" and args.block_id == "source")
    local n = 0 for _ in pairs(args) do n = n + 1 end assert(n == 2)
    if description_throws then error(description_throws, 0) end
    return description, description_error
  end, close_window = function(args) return close("session.close_window", args) end,
  new_window = function(args)
    created = args
    if window_error then return nil, window_error end
    return { window_id = "new", block_ids = { "tool" } }
  end },
}
dofile("config/rex/init.lua")
local expected = {}
local function expect(key, action, args) expected[key] = { action = action, args = args } end
local prefix = "ctrl+[KeyA]>"
for key, action in pairs({
  ["[KeyC]"] = "client.tab.new", ["[KeyB]"] = "client.tab.previous", ["[KeyN]"] = "client.tab.next",
  ["[Comma]"] = "client.tab.rename", ["shift+[KeyN]"] = "session.new", ["shift+[KeyW]"] = "session.rename",
  ["[KeyU]"] = "session.switch", ["shift+[KeyT]"] = "session.switch", ["[BracketLeft]"] = "session.previous",
  ["[BracketRight]"] = "session.next", ["[KeyM]"] = "pane.zoom", ["[KeyF]"] = "pane.zoom",
  ["[Space]"] = "pane.focus_previous", ["[Enter]"] = "pane.focus_previous", ["shift+[KeyO]"] = "client.palette.toggle",
  ["shift+[KeyP]"] = "pane.go_to_directory", ["shift+[KeyF]"] = "client.find.open",
  ["shift+[KeyB]"] = "client.sidebar.activity.toggle", ["shift+[KeyV]"] = "client.sidebar.activity.toggle",
  ["[KeyG]"] = "git", ["shift+[KeyY]"] = "clip",
}) do expect(prefix .. key, action) end
expect(prefix .. "ctrl+[KeyA]", "pane.send_key", { key = "ctrl+a" })
expect(prefix .. "[KeyW]", "dots_close", { target = "window" })
expect(prefix .. "shift+[Backslash]", "pane.split", { direction = "right" })
expect(prefix .. "[Minus]", "pane.split", { direction = "down" })
expect(prefix .. "shift+[Minus]", "pane.split", { direction = "down" })
expect("ctrl+[KeyO]", "client.palette.toggle")
expect("ctrl+[KeyW]", "dots_close", { target = "pane" })
for key, direction in pairs({ H = "left", J = "down", K = "up", L = "right" }) do
  expect(prefix .. "[Key" .. key .. "]", "pane.resize", { direction = direction, amount = 5 })
  expect("ctrl+[Key" .. key .. "]", "pane.focus." .. direction)
  expect("ctrl+shift+[Key" .. key .. "]", "pane.resize", { direction = direction, amount = 5 })
end
for index = 1, 9 do
  expect(prefix .. "[Digit" .. index .. "]", "client.tab.goto", { index = index })
  expect("ctrl+[Digit" .. index .. "]", "client.tab.goto", { index = index })
end
expect("ctrl+shift+[ArrowLeft]", "client.tab.move", { direction = "backward" })
expect("ctrl+shift+[ArrowRight]", "client.tab.move", { direction = "forward" })
local function equal(a, b)
  if type(a) ~= type(b) then return false end
  if type(a) ~= "table" then return a == b end
  for k, v in pairs(a) do if not equal(v, b[k]) then return false end end
  for k in pairs(b) do if a[k] == nil then return false end end
  return true
end
local count = 0
for key, binding in pairs(expected) do
  count = count + 1
  assert(equal(bindings[key], binding), key)
end
assert(count == 60)
for key in pairs(bindings) do assert(expected[key], "unexpected binding: " .. key) end
count = 0
for name in pairs(actions) do count = count + 1 assert(name == "dots_close" or name == "git" or name == "clip") end
assert(count == 3)
assert(equal(actions.dots_close.args.properties.target.enum, { "pane", "window" }))
local function reset()
  closes, queued, describes = {}, nil, 0
  description, description_error, description_throws = { block = { block_id = "source", window_id = "source-window" } }, nil, nil
  close_error, close_throws = nil, nil
end
for _, target in ipairs({ "pane", "window" }) do
  reset()
  assert(actions.dots_close.run(ctx, { target = target }).revision == 1)
  assert(not queued and #closes == 1 and describes == (target == "window" and 1 or 0))
  assert(equal(closes[1], target == "pane" and { method = "block.close", args = { session_id = "session", block_id = "source" } } or { method = "session.close_window", args = { session_id = "session", window_id = "source-window" } }))
end
local function fails_close(context, args, message, attempts, descriptions)
  closes, queued, describes = {}, nil, 0
  local ok, err = pcall(actions.dots_close.run, context, args)
  assert(not ok and tostring(err):find(message, 1, true), tostring(err))
  assert(not queued and #closes == attempts and describes == descriptions)
end
for _, target in ipairs({ "pane", "window" }) do
  reset()
  for _, bad in ipairs({ false, {}, { session_id = "", block_id = "source" }, { session_id = 2, block_id = "source" }, { session_id = "session", block_id = "" }, { session_id = "session", block_id = false } }) do
    fails_close(bad, { target = target }, "context", 0, 0)
  end
  fails_close(nil, { target = target }, "context", 0, 0)
  close_error = "close API failed"
  fails_close(ctx, { target = target }, close_error, 1, target == "window" and 1 or 0)
  close_error, close_throws = nil, "close API threw"
  fails_close(ctx, { target = target }, close_throws, 1, target == "window" and 1 or 0)
end
reset()
for _, args in ipairs({ false, {}, { target = "other" }, { target = 1 } }) do fails_close(ctx, args, "target", 0, 0) end
fails_close(ctx, nil, "target", 0, 0)
description, description_error = nil, "describe failed"
fails_close(ctx, { target = "window" }, "describe failed", 0, 1)
description_error, description_throws = nil, "describe threw"
fails_close(ctx, { target = "window" }, "describe threw", 0, 1)
description_throws = nil
for _, bad in ipairs({ true, {}, { block = false }, { block = {} }, { block = { block_id = "other", window_id = "source-window" } } }) do
  description = bad
  fails_close(ctx, { target = "window" }, "Malformed", 0, 1)
end
for _, bad in ipairs({ false, "", 42, {} }) do
  description = { block = { block_id = "source", window_id = bad } }
  fails_close(ctx, { target = "window" }, "window context", 0, 1)
end
description = { block = { block_id = "source" } }
fails_close(ctx, { target = "window" }, "window context", 0, 1)
local home = assert(os.getenv("HOME"))
process = { foreground = { cwd = "/repo with spaces/'$(literal)'" } }
for _, name in ipairs({ "git", "clip" }) do
  queued = nil
  assert(actions[name].run(ctx).window_id == "new")
  assert(queued.action == "session.select" and queued.args.session_id == "session" and queued.args.window_id == "new")
  assert(created.focus == false and created.layout.options.cwd == process.foreground.cwd)
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
print("PASS Rex bindings and actions")
