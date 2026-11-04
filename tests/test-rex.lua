local bindings, actions = {}, {}
local process, process_error, window_error, created
rex = {
  bind = function(key, action, args)
    assert(not bindings[key], "duplicate binding: " .. key)
    bindings[key] = { action = action, args = args }
  end,
  action = function(action) actions[action.name] = action end,
  block = { call = function(creator, method, args)
    assert(creator == "com.superlogical.terminal" and method == "process")
    assert(args.block_id == "source")
    return process, process_error
  end },
  layout = { block = function(layout) return layout end },
  session = { new_window = function(args)
    created = args
    if window_error then return nil, window_error end
    return { window_id = "new", block_ids = { "tool" } }
  end },
}
dofile("config/rex/init.lua")
assert(bindings["ctrl+[KeyA]>[KeyC]"].action == "client.tab.new")
assert(bindings["ctrl+[KeyA]>ctrl+[KeyA]"].args.key == "ctrl+a")
assert(bindings["ctrl+[KeyA]>shift+[Backslash]"].args.direction == "right")
assert(bindings["ctrl+[Digit9]"].args.index == 9)
assert(bindings["ctrl+[KeyO]"].action == "client.palette.toggle")
local home = assert(os.getenv("HOME"))
process = { foreground = { cwd = "/repo with spaces/'$(literal)'" } }
for _, name in ipairs({ "git", "clip" }) do
  assert(actions[name].run({ block_id = "source" }).window_id == "new")
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
  local ok, err = pcall(actions.git.run, { block_id = "source" })
  assert(not ok and tostring(err):find(message, 1, true))
end
process, process_error = nil, "source pane disappeared"
fails(process_error)
assert(not created)
process, process_error = {}, nil
fails("Cannot determine")
assert(not created)
process = { child = { cwd = "/fallback" } }
assert(actions.git.run({ block_id = "source" }))
assert(created.layout.options.cwd == "/fallback")
process.foreground = { pid = 42, user_id = 0, name = "sudo" }
assert(actions.git.run({ block_id = "source" }))
assert(created.layout.options.cwd == "/fallback")
window_error = "window creation failed"
fails(window_error)
print("PASS Rex bindings and actions")
