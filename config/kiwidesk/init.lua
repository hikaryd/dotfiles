local directory = assert(debug.getinfo(1, "S").source:match("^@(.*/)"))
local theme = dofile(directory .. "theme.lua")
local spaces = { "1", "2", "3", "4", "e", "q", "t", "w", "d" }

for _, space in ipairs(spaces) do
  KiwiDesk.set_mode(space, "bsp")
  KiwiDesk.bind("alt+" .. space, function() KiwiDesk.focus_space(space) end)
  KiwiDesk.bind("alt+shift+" .. space, function() KiwiDesk.move_to_space(space) end)
end
KiwiDesk.set_fallback_space("1")
KiwiDesk.set_gap_global(0)
KiwiDesk.set_space_transition("none")
KiwiDesk.set_refusal_sound(false)
KiwiDesk.set_mode_cue(false)
bsp.set_strategy("longest_side")
mouse.set_hover_focus(true)
mouse.set_follows_focus(false)
scroll_gesture.set_pan("")
scroll_gesture.set_space_step("")
animations.set_on_scrolling(false)
animations.set_on_window_resize(false)
animations.set_on_window_swap(false)
animations.set_on_relayout(false)
animations.set_on_state_change(false)

local current, previous = KiwiDesk.get_state().active_space, nil
KiwiDesk.on("space_change", function(space)
  if space ~= current then previous, current = current, space end
end)
KiwiDesk.bind("alt+tab", function()
  if previous then KiwiDesk.focus_space(previous) end
end)
for key, direction in pairs({ h = "left", j = "down", k = "up", l = "right" }) do
  KiwiDesk.bind("alt+" .. key, function() KiwiDesk.focus(direction) end)
  KiwiDesk.bind("alt+shift+" .. key, function() KiwiDesk.swap(direction) end)
end
KiwiDesk.bind("alt+shift+equal", function() KiwiDesk.resize("x", 50) end)
KiwiDesk.bind("alt+shift+minus", function() KiwiDesk.resize("x", -50) end)
KiwiDesk.bind("alt+space", function() KiwiDesk.toggle_floating() end)
KiwiDesk.bind("cmd+shift+f", function() KiwiDesk.toggle_floating() end)
KiwiDesk.bind("alt+f", function() KiwiDesk.toggle_maximized() end)
-- shortcut: Monocle replaces the whole-space stack, not Rift's nested join groups.
KiwiDesk.set_mode_cycle({ "bsp", "monocle" })
KiwiDesk.bind("cmd+shift+j", function() KiwiDesk.cycle_mode() end)
KiwiDesk.bind("cmd+shift+q", function()
  KiwiDesk.exec([[osascript -e 'tell application id "com.kiwicanopy.kiwidesk" to quit']])
end)
KiwiDesk.bind("ctrl+alt+shift+r", function() KiwiDesk.reload_config() end)

app_rules = {
  ["net.waterfox.waterfox"] = "1",
  ["org.chromium.Thorium"] = "1",
  ["rocks.spotifast.Spotifast"] = "2",
  ["com.tdesktop.Telegram"] = "3",
  ["ru.keepcoder.Telegram"] = "3",
  ["ru.unlimitedtech.express.desktop"] = "3",
  ["ch.protonmail.desktop"] = "3",
  ["com.jpochyla.psst"] = "3",
  ["com.apple.iWork.Pages"] = "4",
  ["de.tutao.tutanota"] = "4",
  ["com.getupnote.desktop"] = "4",
  ["com.apple.systempreferences"] = "e",
  ["com.apple.AppStore"] = "e",
  ["com.docker.docker"] = "q",
  ["com.electron.dockerdesktop"] = "q",
  ["org.jkiss.dbeaver.core.product"] = "q",
  ["com.electron.kontena-lens"] = "q",
  ["io.beekeeperstudio.app"] = "q",
  ["at.eggerapps.Postico"] = "q",
  ["kontur.talk"] = "t",
  ["com.apple.Music"] = "t",
  ["com.apple.Preview"] = "w",
  ["com.microsoft.Outlook"] = "w",
  ["com.superlogical.rex"] = "d",
  ["com.mitchellh.ghostty"] = "d",
  ["com.raphaelamorim.rio"] = "d",
}
local named_spaces = { Ora = "1", Comet = "1", V2BOX = "2", Keet = "2" }
local browsers = {
  ["com.apple.safari"] = true,
  ["com.brave.browser"] = true,
  ["company.thebrowser.browser"] = true,
}
local settings_titles = {}
for _, title in ipairs({
  "General", "Tab", "Tabs", "Password", "Passwords", "Website", "Websites",
  "Extension", "Extensions", "AutoFill", "Search", "Security", "Privacy",
  "Advance", "Advanced", "Preferences", "Settings", "Настройки",
}) do settings_titles[title] = true end
-- shortcut: exact title rules run on creation/focus, not on every background title change.
local function apply_title_rules(id)
  local state = KiwiDesk.get_state()
  for _, window in ipairs(state.windows) do
    if window.id == id then
      local bundle = (window.bundle_id or ""):lower()
      local destination
      if (browsers[bundle] or window.app == "Helium") and settings_titles[window.title] then
        destination = "1"
      elseif bundle == "abnerworks.typora" and (window.title or ""):find("Открыть", 1, true) then
        destination = "4"
      end
      if destination then
        if not window.floating then KiwiDesk.make_floating(id) end
        for _, space in ipairs(state.spaces) do
          for _, member in ipairs(space.windows) do
            if member == id then
              if tostring(space.id) ~= destination then KiwiDesk.move_to_space(destination, id) end
              return
            end
          end
        end
      end
      return
    end
  end
end
-- Names cover apps not installed here, without guessing their bundle identifiers.
KiwiDesk.on("window_created", function(window, app, space, reason)
  if reason ~= "new" then return end
  if named_spaces[app] and named_spaces[app] ~= space then KiwiDesk.move_to_space(named_spaces[app], window) end
  if app == "v2RayTun" then KiwiDesk.make_floating(window) end
  apply_title_rules(window)
end)
KiwiDesk.on("focus_change", function(window) apply_title_rules(window) end)
for _, window in ipairs(KiwiDesk.get_state().windows) do apply_title_rules(window.id) end
float_rules = {
  "com.1password.1password",
  "com.apple.finder",
  "com.cisco.secureclient.gui",
  "com.tinycast.app",
  "com.apple.controlcenter",
  "abnerworks.Typora:Открыть",
}

drag.set_ghost_enabled(false)
drag.set_drop_zone_enabled(false)
border.set_enabled(false)
space_bar.set_edge("top")
space_bar.set_visibility("always")
space_bar.set_reserve(true)
space_bar.set_hide_empty(false)
space_bar.set_item_label("identifier")
space_bar.set_inactive_content("count")
space_bar.set_glyph_span(1)
space_bar.set_show_front_app(false)
space_bar.set_sticky_badge(false)
space_bar.set_active_indicator("edge_mark")
app_bar.set_edge("top")
app_bar.set_title_cap(12)
app_bar.set_active_indicator("edge_mark")
monocle.set_app_bar_visibility("always")
scroll.set_app_bar_visibility("hidden")
kiwishelf.set_alignment("center")
kiwishelf.set_thickness(26)
kiwishelf.set_glyph_size(20)
kiwishelf.set_outer_margin(6)
kiwishelf.set_inner_margin(6)
kiwishelf.set_item_gap(5)
kiwishelf.set_background_style("plain")
kiwishelf.set_background_fit("hug")
kiwishelf.set_liquid_glass(false)
kiwishelf.set_corner_roundness(40)
kiwishelf.set_border(true)
kiwishelf.set_border_width(1)
kiwishelf.set_highlight_width(1)
kiwishelf.set_font_family("System Monospaced")
kiwishelf.set_font_size(12)
kiwishelf.set_font_weight("medium")
kiwishelf.set_icon_source("app_font")
kiwishelf.set_pocket_visibility("hidden")
kiwishelf.set_fill_color(theme.fill)
kiwishelf.set_item_color(theme.item)
kiwishelf.set_active_item_color(theme.active)
kiwishelf.set_highlight_color(theme.highlight)
kiwishelf.set_hover_fill_color(theme.hover_fill)
kiwishelf.set_hover_item_color(theme.hover_item)
kiwishelf.set_border_color(theme.border)
kiwishelf.set_group_badge_color(theme.badge)
kiwishelf.set_group_badge_text_color(theme.badge_text)
space_bar.set_focused_item_color(theme.focused)
