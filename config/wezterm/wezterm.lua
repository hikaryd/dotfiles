local wezterm = require 'wezterm'
local act = wezterm.action
local mux = wezterm.mux
local config = wezterm.config_builder()
local home = wezterm.home_dir

-- =============================================================================
-- Plugin: resurrect (session persistence, replaces tmux-resurrect + continuum)
-- =============================================================================
local has_resurrect, resurrect = pcall(wezterm.plugin.require,
  'https://github.com/MLFlexer/resurrect.wezterm')

if has_resurrect then
  resurrect.state_manager.periodic_save({
    interval_seconds = 300,
    save_workspaces = true,
    save_windows = true,
  })

  -- Write current_state on every periodic save so gui-startup can restore
  wezterm.on('resurrect.state_manager.periodic_save.finished', function()
    local ws = mux.get_active_workspace()
    resurrect.state_manager.write_current_state(ws, 'workspace')
  end)
end

-- =============================================================================
-- Appearance (from Ghostty config)
-- =============================================================================
config.color_scheme = 'Catppuccin Mocha'
config.colors = {
  background = '#0B0E14',
  cursor_bg = '#cdd6f4',
  cursor_fg = '#0B0E14',
  cursor_border = '#cdd6f4',
  tab_bar = {
    background = '#0B0E14',
    new_tab = { bg_color = '#0B0E14', fg_color = '#585b70' },
    new_tab_hover = { bg_color = '#0B0E14', fg_color = '#FFC799' },
  },
}
config.font = wezterm.font('Iosevka')
config.font_size = 14
config.line_height = 1.45

config.default_cursor_style = 'SteadyBlock'
config.cursor_blink_rate = 0
config.hide_mouse_cursor_when_typing = true

config.window_close_confirmation = 'NeverPrompt'
config.window_decorations = 'RESIZE'
config.window_background_opacity = 0.95
config.macos_window_background_blur = 10
config.window_padding = { left = 6, right = 6, top = 6, bottom = 6 }

-- Dim inactive panes
config.inactive_pane_hsb = {
  saturation = 0.8,
  brightness = 0.6,
}

config.scrollback_lines = 1000000

-- macOS Option key sends composed characters (not Alt), matching Ghostty
config.send_composed_key_when_left_alt_is_pressed = true
config.send_composed_key_when_right_alt_is_pressed = true

-- =============================================================================
-- Tab bar — minimal powerline (replaces tmux-dotbar)
-- =============================================================================
config.use_fancy_tab_bar = false
config.tab_bar_at_bottom = false
config.show_new_tab_button_in_tab_bar = false
config.tab_max_width = 32
config.status_update_interval = 500

local C = {
  bg     = '#0B0E14',
  active = '#FFC799',
  dim    = '#585b70',
}

-- =============================================================================
-- Shell
-- =============================================================================
config.default_prog = { '/opt/homebrew/bin/zsh', '-l' }

-- =============================================================================
-- Leader key: Ctrl-A (replaces tmux prefix)
-- =============================================================================
config.leader = { key = 'a', mods = 'CTRL', timeout_milliseconds = 1000 }

-- =============================================================================
-- Workspaces (replaces sesh.toml named sessions)
-- =============================================================================
local workspaces = {
  { name = 'dots', cwd = home .. '/dots' },
  { name = 'ods', cwd = home .. '/Documents/dev/operational-data-store' },
  { name = 'bff', cwd = home .. '/Documents/dev/bff' },
  { name = 'jobboard-scraper', cwd = home .. '/Documents/dev/jobboard-scraper' },
  { name = 'x5-podbor-ods', cwd = home .. '/Documents/dev/x5-podbor-ods' },
}

-- =============================================================================
-- State tracking (last tab / last workspace)
-- =============================================================================
local prev_tab = {}
local last_tab = {}
local prev_ws = {}
local last_ws = {}

-- Cached padding value (only recalculated on resize)
local cached_pad = {}

wezterm.on('window-resized', function(window, pane)
  local wid = window:window_id()
  local win_dims = window:get_dimensions()
  local pane_dims = pane:get_dimensions()
  local cell_w = pane_dims.pixel_width / math.max(pane_dims.cols, 1)
  cached_pad[wid] = cell_w > 0 and math.floor(win_dims.pixel_width / cell_w) or pane_dims.cols
end)

wezterm.on('update-status', function(window, pane)
  local wid = window:window_id()

  -- Track last tab
  local tid = window:active_tab():tab_id()
  if prev_tab[wid] and prev_tab[wid] ~= tid then
    last_tab[wid] = prev_tab[wid]
  end
  prev_tab[wid] = tid

  -- Track last workspace
  local ws = window:active_workspace()
  if prev_ws[wid] and prev_ws[wid] ~= ws then
    last_ws[wid] = prev_ws[wid]
  end
  prev_ws[wid] = ws

  -- Centering: use cached total_cols from resize event
  local total_cols = cached_pad[wid]
  if not total_cols then
    local win_dims = window:get_dimensions()
    local pane_dims = pane:get_dimensions()
    local cell_w = pane_dims.pixel_width / math.max(pane_dims.cols, 1)
    total_cols = cell_w > 0 and math.floor(win_dims.pixel_width / cell_w) or pane_dims.cols
    cached_pad[wid] = total_cols
  end

  local total_tw = 0
  for i, t in ipairs(window:mux_window():tabs()) do
    local title = t:active_pane():get_title()
    if #title > 20 then title = title:sub(1, 20) .. '..' end
    total_tw = total_tw + 4 + #tostring(i) + #title
  end

  local right_w = #ws + 3
  local pad = math.max(0, math.floor((total_cols - total_tw - right_w) / 2))

  window:set_left_status(wezterm.format {
    { Background = { Color = C.bg } },
    { Text = string.rep(' ', pad) },
  })

  -- Right status
  local right = {}
  if window:leader_is_active() then
    table.insert(right, { Background = { Color = C.bg } })
    table.insert(right, { Foreground = { Color = C.active } })
    table.insert(right, { Attribute = { Intensity = 'Bold' } })
    table.insert(right, { Text = ' LEADER ' })
  end
  table.insert(right, { Background = { Color = C.bg } })
  table.insert(right, { Foreground = { Color = C.dim } })
  table.insert(right, { Text = ' ' .. ws .. ' ' })
  window:set_right_status(wezterm.format(right))
end)

-- Helper: consistent tab title
local function get_tab_title(tab_info)
  local title = tab_info.active_pane.title
  if #title > 20 then title = title:sub(1, 20) .. '..' end
  return title
end

-- Tab title: plain text, active highlighted
wezterm.on('format-tab-title', function(tab)
  local idx = tab.tab_index + 1
  local title = get_tab_title(tab)
  local fg = tab.is_active and C.active or C.dim

  return wezterm.format {
    { Background = { Color = C.bg } },
    { Foreground = { Color = fg } },
    { Attribute = { Intensity = tab.is_active and 'Bold' or 'Normal' } },
    { Text = string.format(' %d: %s ', idx, title) },
  }
end)

-- =============================================================================
-- Auto-restore on startup (resurrect)
-- =============================================================================
if has_resurrect then
  wezterm.on('gui-startup', function()
    -- Restore ALL saved workspaces, not just current_state
    local state_dir = resurrect.state_manager.save_state_dir .. 'workspace'
    local ok, files = pcall(wezterm.read_dir, state_dir)
    if not ok then return end

    local restored = {}
    for _, path in ipairs(files) do
      local name = path:match('([^/]+)%.json$')
      if name then
        local state = resurrect.state_manager.load_state(name, 'workspace')
        if state and state.workspace then
          resurrect.workspace_state.restore_workspace(state, {
            spawn_in_workspace = true,
            relative = true,
            restore_text = true,
            on_pane_restore = resurrect.tab_state.default_on_pane_restore,
          })
          table.insert(restored, name)
        end
      end
    end

    -- Switch to workspace from current_state (last active)
    local cs_path = resurrect.state_manager.save_state_dir .. 'current_state'
    local f = io.open(cs_path, 'r')
    if f then
      local active_ws = f:read('*line')
      f:close()
      if active_ws then
        mux.set_active_workspace(active_ws)
      end
    end

    wezterm.log_info('Restored workspaces: ' .. table.concat(restored, ', '))
  end)
end

-- =============================================================================
-- Helper functions
-- =============================================================================
local function get_cwd(pane)
  local url = pane:get_current_working_dir()
  if url then return url.file_path end
  return home
end

local function switch_to_last_tab(window, pane)
  local target = last_tab[window:window_id()]
  if not target then return end
  for _, t in ipairs(window:mux_window():tabs()) do
    if t:tab_id() == target then
      t:activate()
      return
    end
  end
end

local function switch_to_last_workspace(window, pane)
  local target = last_ws[window:window_id()]
  if target then
    window:perform_action(act.SwitchToWorkspace { name = target }, pane)
  else
    window:toast_notification('WezTerm', 'No previous workspace', nil, 2000)
  end
end

local function switch_workspace_relative(direction)
  return wezterm.action_callback(function(window, pane)
    local names = mux.get_workspace_names()
    if #names < 2 then return end
    local current = window:active_workspace()
    for i, name in ipairs(names) do
      if name == current then
        local idx = ((i - 1 + direction) % #names) + 1
        window:perform_action(
          act.SwitchToWorkspace { name = names[idx] }, pane)
        return
      end
    end
  end)
end

local function workspace_picker(window, pane)
  local choices = {}
  local seen = {}

  -- 1. Active workspaces (marked with *)
  for _, name in ipairs(mux.get_workspace_names()) do
    seen[name] = true
    table.insert(choices, { id = name, label = name .. '  *' })
  end

  -- 2. Predefined workspaces from config
  for _, ws in ipairs(workspaces) do
    if not seen[ws.name] then
      seen[ws.name] = true
      table.insert(choices, { id = ws.cwd, label = ws.name })
    end
  end

  -- 3. Scan ~/Documents/dev/ for project directories
  local dev_dir = home .. '/Documents/dev'
  local ok, dirs = pcall(wezterm.read_dir, dev_dir)
  if ok then
    for _, full_path in ipairs(dirs) do
      local name = full_path:match('([^/]+)$')
      if name and not seen[name] then
        seen[name] = true
        table.insert(choices, { id = full_path, label = name })
      end
    end
  end

  window:perform_action(act.InputSelector {
    action = wezterm.action_callback(function(win, p, id, label)
      if not id then return end
      local name = label:gsub('  %*$', '')
      -- Active workspace — just switch
      for _, n in ipairs(mux.get_workspace_names()) do
        if n == name then
          win:perform_action(act.SwitchToWorkspace { name = name }, p)
          return
        end
      end
      -- New workspace with cwd
      win:perform_action(act.SwitchToWorkspace {
        name = name,
        spawn = { cwd = id },
      }, p)
    end),
    fuzzy = true,
    title = 'Switch Workspace',
    choices = choices,
  }, pane)
end

-- =============================================================================
-- Keybindings
-- =============================================================================
config.keys = {
  -- ── No-prefix bindings ──────────────────────────────────────────────────

  -- Pane navigation: Ctrl-H/J/K/L
  { key = 'h', mods = 'CTRL', action = act.ActivatePaneDirection('Left') },
  { key = 'j', mods = 'CTRL', action = act.ActivatePaneDirection('Down') },
  { key = 'k', mods = 'CTRL', action = act.ActivatePaneDirection('Up') },
  { key = 'l', mods = 'CTRL', action = act.ActivatePaneDirection('Right') },

  -- Tab selection: Ctrl-1..9
  { key = '1', mods = 'CTRL', action = act.ActivateTab(0) },
  { key = '2', mods = 'CTRL', action = act.ActivateTab(1) },
  { key = '3', mods = 'CTRL', action = act.ActivateTab(2) },
  { key = '4', mods = 'CTRL', action = act.ActivateTab(3) },
  { key = '5', mods = 'CTRL', action = act.ActivateTab(4) },
  { key = '6', mods = 'CTRL', action = act.ActivateTab(5) },
  { key = '7', mods = 'CTRL', action = act.ActivateTab(6) },
  { key = '8', mods = 'CTRL', action = act.ActivateTab(7) },
  { key = '9', mods = 'CTRL', action = act.ActivateTab(8) },

  -- Move tab: Ctrl-Shift-Left/Right
  { key = 'LeftArrow',  mods = 'CTRL|SHIFT', action = act.MoveTabRelative(-1) },
  { key = 'RightArrow', mods = 'CTRL|SHIFT', action = act.MoveTabRelative(1) },

  -- Close pane: Ctrl-W
  { key = 'w', mods = 'CTRL', action = act.CloseCurrentPane { confirm = false } },

  -- ── Leader+SHIFT bindings ────────────────────────────────────────────────

  -- Workspace picker: Leader+T
  { key = 't', mods = 'LEADER|SHIFT', action = wezterm.action_callback(workspace_picker) },
  -- Resurrect save: Leader+S
  { key = 's', mods = 'LEADER|SHIFT', action = wezterm.action_callback(function(win, pane)
    if has_resurrect then
      local state = resurrect.workspace_state.get_workspace_state()
      resurrect.state_manager.save_state(state)
      resurrect.state_manager.write_current_state(state.workspace, 'workspace')
      win:toast_notification('WezTerm', 'Session saved', nil, 3000)
    end
  end) },

  -- ── Leader bindings (Ctrl-A + key) ──────────────────────────────────────

  -- Splits (preserve cwd)
  { key = '|', mods = 'LEADER', action = act.SplitPane { direction = 'Right' } },
  { key = '_', mods = 'LEADER', action = act.SplitPane { direction = 'Down' } },

  -- New tab
  { key = 'c', mods = 'LEADER', action = act.SpawnTab('CurrentPaneDomain') },

  -- Resize panes: Leader + h/j/k/l
  { key = 'h', mods = 'LEADER', action = act.AdjustPaneSize { 'Left', 5 } },
  { key = 'j', mods = 'LEADER', action = act.AdjustPaneSize { 'Down', 5 } },
  { key = 'k', mods = 'LEADER', action = act.AdjustPaneSize { 'Up', 5 } },
  { key = 'l', mods = 'LEADER', action = act.AdjustPaneSize { 'Right', 5 } },

  -- Zoom pane toggle
  { key = 'm', mods = 'LEADER', action = act.TogglePaneZoomState },

  -- Copy mode (vi keys)
  { key = 'v', mods = 'LEADER', action = act.ActivateCopyMode },

  -- Close tab
  { key = 'w', mods = 'LEADER', action = act.CloseCurrentTab { confirm = false } },

  -- Last tab (toggle)
  { key = 'Space', mods = 'LEADER', action = wezterm.action_callback(switch_to_last_tab) },

  -- Previous tab
  { key = 'b', mods = 'LEADER', action = act.ActivateTabRelative(-1) },

  -- Workspace: prev / next
  { key = '[', mods = 'LEADER', action = switch_workspace_relative(-1) },
  { key = ']', mods = 'LEADER', action = switch_workspace_relative(1) },

  -- Workspace: last (Leader+Enter)
  { key = 'Enter', mods = 'LEADER', action = wezterm.action_callback(switch_to_last_workspace) },

  -- Lazygit in new tab
  { key = 'g', mods = 'LEADER', action = wezterm.action_callback(function(window, pane)
    window:perform_action(act.SpawnCommandInNewTab {
      args = { '/opt/homebrew/bin/lazygit', '--use-config-file', home .. '/.config/lazygit/config.yml' },
      cwd = get_cwd(pane),
      set_environment_variables = { PATH = '/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin' },
    }, pane)
  end) },

  -- Reload config
  { key = 'r', mods = 'LEADER', action = act.ReloadConfiguration },
}

return config
