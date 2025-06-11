local wezterm = require("wezterm")
local config = wezterm.config_builder()
local act = wezterm.action
local mux = wezterm.mux

-- Base Configuration
config.alternate_buffer_wheel_scroll_speed = 2
config.scrollback_lines = 10000000

-- Font Configuration
config.font = wezterm.font({
	family = "Monaspace Neon",
	-- family = "Monaspace Argon",
	-- family='Monaspace Xenon',
	-- family='Monaspace Radon',
	-- family='Monaspace Krypton',
})
config.bold_brightens_ansi_colors = true
config.hide_mouse_cursor_when_typing = true
config.freetype_load_flags = "NO_HINTING"

-- Cursor Configuration
config.default_cursor_style = "SteadyBar"

-- Color Configuration
config.force_reverse_video_cursor = true

-- Window Configuration
config.window_background_opacity = 0.8
config.initial_rows = 45
config.initial_cols = 180
config.macos_window_background_blur = 20
config.window_decorations = "RESIZE"
config.window_close_confirmation = "NeverPrompt"

-- Performance Settings
config.max_fps = 250
config.animation_fps = 120
config.cursor_blink_rate = 1000

-- Tab Bar Configuration
config.enable_tab_bar = true
config.hide_tab_bar_if_only_one_tab = true
config.show_tab_index_in_tab_bar = false
config.use_fancy_tab_bar = false

-- Default Shell Configuration
config.default_prog = { "/etc/profiles/per-user/tronin.egor/bin/nu" }

-- Tab Configuration
-- local tabline = wezterm.plugin.require("https://github.com/michaelbrusegard/tabline.wez")
-- tabline.setup({
-- 	options = {
-- 		icons_enabled = true,
-- 		theme = "Catppuccin Mocha",
-- 		tabs_enabled = true,
-- 		theme_overrides = {},
-- 		section_separators = {
-- 			left = wezterm.nerdfonts.ple_right_half_circle_thick,
-- 			right = wezterm.nerdfonts.ple_left_half_circle_thick,
-- 		},
-- 		component_separators = {
-- 			left = wezterm.nerdfonts.ple_right_half_circle_thin,
-- 			right = wezterm.nerdfonts.ple_left_half_circle_thin,
-- 		},
-- 		tab_separators = {
-- 			left = wezterm.nerdfonts.ple_right_half_circle_thick,
-- 			right = wezterm.nerdfonts.ple_left_half_circle_thick,
-- 		},
-- 	},
-- 	sections = {
-- 		-- tabline_a = { 'mode' },
-- 		tabline_b = { "workspace" },
-- 		tabline_c = { " " },
-- 		tab_active = {
-- 			"index",
-- 			{ "parent", padding = 0 },
-- 			"/",
-- 			{ "cwd", padding = { left = 0, right = 1 } },
-- 			{ "zoomed", padding = 0 },
-- 		},
-- 		tab_inactive = { "index", { "process", padding = { left = 0, right = 1 } } },
-- 		tabline_x = { "ram", "cpu" },
-- 		-- tabline_y = { 'datetime', 'battery' },
-- 		-- tabline_z = { 'domain' },
-- 	},
-- 	extensions = {},
-- })
-- tabline.apply_to_config(config)

local keys = {
	{
		key = "v",
		mods = "CTRL",
		action = act.SplitPane({
			direction = "Right",
			size = { Percent = 50 },
		}),
	},
	{
		key = "v",
		mods = "CTRL|SHIFT",
		action = act.SplitPane({
			direction = "Down",
			size = { Percent = 50 },
		}),
	},
	{
		key = "w",
		mods = "CTRL",
		action = act.CloseCurrentPane({ confirm = false }),
	},
	{
		key = "x",
		mods = "CTRL",
		action = act.PaneSelect({ mode = "SwapWithActiveKeepFocus" }),
	},
	{
		key = "z",
		mods = "CMD",
		action = act.TogglePaneZoomState,
	},
	{
		key = "w",
		mods = "CTRL|SHIFT",
		action = act.CloseCurrentTab({ confirm = false }),
	},
	{
		key = "h",
		mods = "CTRL",
		action = act.ActivatePaneDirection("Left"),
	},
	{
		key = "j",
		mods = "CTRL",
		action = act.ActivatePaneDirection("Down"),
	},
	{
		key = "k",
		mods = "CTRL",
		action = act.ActivatePaneDirection("Up"),
	},
	{
		key = "l",
		mods = "CTRL",
		action = act.ActivatePaneDirection("Right"),
	},
	{
		key = "h",
		mods = "CTRL|SHIFT",
		action = act.AdjustPaneSize({ "Left", 10 }),
	},
	{
		key = "j",
		mods = "CTRL|SHIFT",
		action = act.AdjustPaneSize({ "Down", 10 }),
	},
	{
		key = "k",
		mods = "CTRL|SHIFT",
		action = act.AdjustPaneSize({ "Up", 10 }),
	},
	{
		key = "l",
		mods = "CTRL|SHIFT",
		action = act.AdjustPaneSize({ "Right", 10 }),
	},
}

-- tab config
config.hide_tab_bar_if_only_one_tab = false
config.use_fancy_tab_bar = false

local function get_current_working_dir(tab)
	local current_dir = tab.active_pane and tab.active_pane.current_working_dir or { file_path = "" }
	local HOME_DIR = string.format("file://%s", os.getenv("HOME"))

	return current_dir == HOME_DIR and "." or string.gsub(current_dir.file_path, "(.*[/\\])(.*)", "%2")
end

-- tab title
wezterm.on("format-tab-title", function(tab, tabs, panes, config, hover, max_width)
	local has_unseen_output = false
	if not tab.is_active then
		for _, pane in ipairs(tab.panes) do
			if pane.has_unseen_output then
				has_unseen_output = true
				break
			end
		end
	end

	local cwd = wezterm.format({
		{ Attribute = { Intensity = "Bold" } },
		{ Text = get_current_working_dir(tab) },
	})

	local title = string.format(" [%s] %s", tab.tab_index + 1, cwd)

	if has_unseen_output then
		return {
			{ Foreground = { Color = "#8866bb" } },
			{ Text = title },
		}
	end

	return {
		{ Text = title },
	}
end)

-- workspaces
wezterm.on("update-right-status", function(window, pane)
	window:set_right_status(window:active_workspace())
end)
local workspace_switcher = wezterm.plugin.require("https://github.com/MLFlexer/smart_workspace_switcher.wezterm")
workspace_switcher.zoxide_path = "/opt/homebrew/bin/zoxide"
wezterm.on("gui-startup", function(cmd)
	local dotfiles_path = wezterm.home_dir .. "/dotfiles"
	local dev_path = wezterm.home_dir .. "/Documents/dev"
	local tab, build_pane, window = mux.spawn_window({
		workspace = "dotfiles",
		cwd = dotfiles_path,
	})
	build_pane:send_text("nvim\n")
	local tab, build_pane, window = mux.spawn_window({
		workspace = "~/Documents/dev",
		cwd = dev_path,
	})
	mux.set_active_workspace("~/Documents/dev")
end)
table.insert(keys, { key = "s", mods = "CTRL|SHIFT", action = workspace_switcher.switch_workspace() })
table.insert(keys, { key = "t", mods = "CTRL|SHIFT", action = act.ShowLauncherArgs({ flags = "FUZZY|WORKSPACES" }) })
table.insert(keys, { key = "c", mods = "CTRL|SHIFT", action = act.SwitchToWorkspace({ name = "dotfiles" }) })
table.insert(keys, { key = "d", mods = "CTRL|SHIFT", action = act.SwitchToWorkspace({ name = "~/Documents/dev" }) })
table.insert(keys, { key = "[", mods = "CTRL|SHIFT", action = act.SwitchWorkspaceRelative(1) })
table.insert(keys, { key = "]", mods = "CTRL|SHIFT", action = act.SwitchWorkspaceRelative(-1) })

config.keys = keys

config.window_padding = {
	left = "1cell",
	right = "1cell",
	top = "0.5cell",
	bottom = "0.5cell",
}

config.audible_bell = "Disabled"
config.visual_bell = {
	fade_in_function = "EaseIn",
	fade_in_duration_ms = 150,
	fade_out_function = "EaseOut",
	fade_out_duration_ms = 150,
}

return config
