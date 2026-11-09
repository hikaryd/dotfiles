local bindings, events, calls, settings, modes = {}, {}, {}, {}, {}
local active, windows, workspace_state = "1", {}, {}
local function record(name, ...)
  local args = { ... }
  calls[#calls + 1] = { name, table.unpack(args) }
  settings[name] = args
end
local function api(namespace, names)
  local object = {}
  for name in names:gmatch("%S+") do
    object[name] = function(...) record(namespace .. "." .. name, ...) end
  end
  return object
end
KiwiDesk = api("KiwiDesk", "set_fallback_space set_gap_global set_space_transition set_refusal_sound set_mode_cue set_mode_cycle move_to_space make_floating focus swap resize toggle_floating toggle_maximized cycle_mode exec reload_config")
KiwiDesk.set_mode = function(space, mode)
  assert(not modes[space], "duplicate space")
  modes[space] = mode
end
KiwiDesk.bind = function(key, callback)
  assert(not bindings[key], "duplicate binding: " .. key)
  bindings[key] = callback
end
KiwiDesk.on = function(event, callback) events[event] = callback end
KiwiDesk.get_state = function() return { active_space = active, windows = windows, spaces = workspace_state } end
KiwiDesk.focus_space = function(space)
  record("KiwiDesk.focus_space", space)
  active = space
  events.space_change(space, modes[space])
end
bsp = api("bsp", "set_strategy")
mouse = api("mouse", "set_hover_focus set_follows_focus")
scroll_gesture = api("scroll_gesture", "set_pan set_space_step")
animations = api("animations", "set_on_scrolling set_on_window_resize set_on_window_swap set_on_relayout set_on_state_change")
drag = api("drag", "set_ghost_enabled set_drop_zone_enabled")
border = api("border", "set_enabled")
space_bar = api("space_bar", "set_edge set_visibility set_reserve set_hide_empty set_item_label set_inactive_content set_glyph_span set_show_front_app set_sticky_badge set_active_indicator set_focused_item_color")
app_bar = api("app_bar", "set_edge set_title_cap set_active_indicator")
monocle = api("monocle", "set_app_bar_visibility")
scroll = api("scroll", "set_app_bar_visibility")
kiwishelf = api("kiwishelf", "set_alignment set_thickness set_glyph_size set_outer_margin set_inner_margin set_item_gap set_background_style set_background_fit set_liquid_glass set_corner_roundness set_border set_border_width set_highlight_width set_font_family set_font_size set_font_weight set_icon_source set_pocket_visibility set_fill_color set_item_color set_active_item_color set_highlight_color set_hover_fill_color set_hover_item_color set_border_color set_group_badge_color set_group_badge_text_color")
dofile("config/kiwidesk/init.lua")
local function press(key, name, ...)
  assert(bindings[key], "missing binding: " .. key)
  calls = {}
  bindings[key]()
  local expected = { name, ... }
  if not name then assert(#calls == 0) return end
  assert(#calls == 1, key .. " must execute exactly one action")
  assert(#calls[1] == #expected)
  for i, value in ipairs(expected) do assert(calls[1][i] == value, key .. ": wrong argument " .. i) end
end
press("alt+tab", nil)
local spaces = { "1", "2", "3", "4", "e", "q", "t", "w", "d" }
local count = 0
for _ in pairs(modes) do count = count + 1 end
assert(count == #spaces)
for _, space in ipairs(spaces) do
  assert(modes[space] == "bsp")
  press("alt+" .. space, "KiwiDesk.focus_space", space)
  press("alt+shift+" .. space, "KiwiDesk.move_to_space", space)
end
press("alt+2", "KiwiDesk.focus_space", "2")
press("alt+d", "KiwiDesk.focus_space", "d")
events.space_change("d", "bsp")
press("alt+tab", "KiwiDesk.focus_space", "2")
press("alt+tab", "KiwiDesk.focus_space", "d")
press("alt+tab", "KiwiDesk.focus_space", "2")
for key, direction in pairs({ h = "left", j = "down", k = "up", l = "right" }) do
  press("alt+" .. key, "KiwiDesk.focus", direction)
  press("alt+shift+" .. key, "KiwiDesk.swap", direction)
end
press("alt+shift+equal", "KiwiDesk.resize", "x", 50)
press("alt+shift+minus", "KiwiDesk.resize", "x", -50)
press("alt+space", "KiwiDesk.toggle_floating")
press("cmd+shift+f", "KiwiDesk.toggle_floating")
press("alt+f", "KiwiDesk.toggle_maximized")
press("cmd+shift+j", "KiwiDesk.cycle_mode")
assert(not bindings["ctrl+alt+s"], "saving a profile must not mask future Lua settings")
press("cmd+shift+q", "KiwiDesk.exec", [[osascript -e 'tell application id "com.kiwicanopy.kiwidesk" to quit']])
press("ctrl+alt+shift+r", "KiwiDesk.reload_config")
assert(not bindings["cmd+shift+h"] and not bindings["cmd+shift+l"], "unsupported joins must not pretend to work")
assert(app_rules["de.tutao.tutanota"] == "4")
assert(app_rules["com.docker.docker"] == "q")
assert(app_rules["kontur.talk"] == "t")
for _, space in pairs(app_rules) do assert(modes[space], "unknown app destination") end
local floats = {}
for _, rule in ipairs(float_rules) do floats[rule] = true end
assert(floats["com.apple.finder"] and floats["com.cisco.secureclient.gui"])
assert(not floats["com.apple.Safari:Search"] and not floats["com.brave.Browser:Privacy"])
local function created(app, bundle, title, reason, space)
  calls = {}
  windows = { { id = 99, app = app, bundle_id = bundle, title = title, floating = false } }
  workspace_state = { { id = space or "d", windows = { 99 } } }
  events.window_created(99, app, space or "d", reason or "new", bundle)
end
for app, destination in pairs({ Ora = "1", Comet = "1", V2BOX = "2", Keet = "2" }) do
  created(app, "example.unverified", "ordinary window")
  assert(#calls == 1 and calls[1][1] == "KiwiDesk.move_to_space" and calls[1][2] == destination and calls[1][3] == 99)
  created(app, "example.unverified", "ordinary window", "new", destination)
  assert(#calls == 0, "new windows already in their destination must not move again")
  for _, reason in ipairs({ "returned", "restored", "existing" }) do
    created(app, "example.unverified", "ordinary window", reason)
    assert(#calls == 0, "must preserve manually moved returning windows")
  end
end
created("v2RayTun", "example.vpn", "connection")
assert(#calls == 1 and calls[1][1] == "KiwiDesk.make_floating" and calls[1][2] == 99)
for _, title in ipairs({ "Search - Google", "Privacy Policy - Example", "General relativity", "Example Domain" }) do
  created("Safari", "com.apple.Safari", title)
  assert(#calls == 0, "ordinary browser pages must remain tiled: " .. title)
end
for _, browser in ipairs({ { "Safari", "com.apple.Safari" }, { "Brave Browser", "com.brave.Browser" }, { "Arc", "company.thebrowser.Browser" }, { "Helium", "unknown.helium" } }) do
  created(browser[1], browser[2], "Settings")
  assert(#calls == 2 and calls[1][1] == "KiwiDesk.make_floating" and calls[1][2] == 99)
  assert(calls[2][1] == "KiwiDesk.move_to_space" and calls[2][2] == "1" and calls[2][3] == 99)
  windows[1].floating = true
  workspace_state[1].id = "1"
  calls = {}
  events.focus_change(99)
  assert(#calls == 0, "matching dialogs must be idempotent on repeated focus")
end
created("Typora", "abnerworks.Typora", "Открыть документ")
assert(#calls == 2 and calls[2][2] == "4" and calls[2][3] == 99)
created("Unrelated", "example.app", "Settings")
assert(#calls == 0, "unrelated apps must not match browser settings")
created("Safari", "com.apple.Safari", "Loading")
windows[1].title = "Preferences"
events.focus_change(99)
assert(#calls == 2, "a late settings title must apply on focus")
calls = {}
events.focus_change(100)
assert(#calls == 0, "unknown window must not move the focused window")
windows, workspace_state = {}, {}
assert(settings["KiwiDesk.set_gap_global"][1] == 0)
assert(settings["space_bar.set_reserve"][1] and settings["space_bar.set_visibility"][1] == "always")
assert(settings["kiwishelf.set_thickness"][1] == 26 and not settings["kiwishelf.set_liquid_glass"][1])
assert(settings["kiwishelf.set_active_item_color"][1] == "#ffc799")
assert(settings["kiwishelf.set_fill_color"][1] == "#101010F2")
assert(settings["mouse.set_hover_focus"][1] and not settings["mouse.set_follows_focus"][1])
print("PASS KiwiDesk spaces, keybindings, Alt+Tab toggle, app rules and minimal Vesper shelf")
