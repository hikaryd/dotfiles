-- tarmac configuration
-- Based on config/rift/config.toml.
--
-- Tarmac normalizes lettered workspaces to uppercase internally, so the Rift
-- workspaces e/q/t/w/d are configured as E/Q/T/W/D here.

gar.set("mod_key", "option")
gar.set("gap_inner", 0)
gar.set("gap_outer", 0)
gar.set("bar_height", 0)

gar.set("focus_follows_mouse", true)
gar.set("mouse_follows_focus", false)
gar.set("terminal", "open -na Ghostty")

gar.set("border_width", 4)
gar.set("border_color_focused", "#39bae6")
gar.set("border_color_unfocused", "#3e4b59")
gar.set("border_radius", 10)

local workspaces = { "1", "2", "3", "4", "E", "Q", "T", "W", "D" }

for _, workspace in ipairs(workspaces) do
	gar.workspace(workspace, { layout = "bsp", gap_inner = 0, gap_outer = 0 })
end

local function bind_workspace(key, workspace)
	gar.bind("mod+" .. key, "workspace " .. workspace)
	gar.bind("mod+shift+" .. key, "move_to_workspace " .. workspace)
end

bind_workspace("1", "1")
bind_workspace("2", "2")
bind_workspace("3", "3")
bind_workspace("4", "4")
bind_workspace("e", "E")
bind_workspace("q", "Q")
bind_workspace("t", "T")
bind_workspace("w", "W")
bind_workspace("d", "D")

-- Tarmac currently has workspace_next/workspace_prev, but no Rift-style
-- switch_to_last_workspace action.
gar.bind("mod+tab", "workspace_prev")
gar.bind("mod+shift+tab", "workspace_next")

-- Focus navigation.
gar.bind("mod+h", "focus left")
gar.bind("mod+j", "focus down")
gar.bind("mod+k", "focus up")
gar.bind("mod+l", "focus right")

-- Window movement.
gar.bind("mod+shift+h", "swap left")
gar.bind("mod+shift+j", "swap down")
gar.bind("mod+shift+k", "swap up")
gar.bind("mod+shift+l", "swap right")

-- Stack-like controls available in Tarmac. Rift's toggle_stack/join_window
-- commands do not have exact Tarmac equivalents.
gar.bind("command+shift+j", "promote_stack")
gar.bind("command+shift+h", "swap left")
gar.bind("command+shift+l", "swap right")
gar.bind("mod+shift+u", "unstack")

-- Resize. Tarmac resizes directionally, so Equal/Minus map to horizontal grow/shrink.
gar.bind("mod+shift+equal", "resize right")
gar.bind("mod+shift+minus", "resize left")
gar.bind("mod+ctrl+h", "resize left")
gar.bind("mod+ctrl+j", "resize down")
gar.bind("mod+ctrl+k", "resize up")
gar.bind("mod+ctrl+l", "resize right")

-- Floating / service commands.
gar.bind("mod+space", "toggle_float")
gar.bind("command+shift+f", "toggle_float")
gar.bind("mod+shift+r", "reload")
gar.bind("command+shift+q", "exit")

-- Multi-monitor controls.
gar.bind("mod+comma", "focus_monitor_prev")
gar.bind("mod+period", "focus_monitor_next")
gar.bind("mod+shift+comma", "move_to_monitor_prev")
gar.bind("mod+shift+period", "move_to_monitor_next")

local function exact(value)
	return { value = value, mode = "exact" }
end

local function regex(value)
	return { value = value, mode = "regex" }
end

local function rule(name, match, actions)
	match = match or {}
	actions = actions or {}
	gar.rule({
		id = name,
		name = name,
		match = match,
		actions = actions,
	})
end

-- Floating browser settings windows.
rule("float_safari_settings", {
	app_name = exact("Safari"),
	title = regex(
		"^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|curity)|Privacy|Advance|Preferences|Настройки)$"
	),
}, { floating = true, workspace = "1" })

rule("float_brave_settings", {
	app_name = exact("Brave"),
	title = regex(
		"^(General|Tabs?|Passwords?|Websites?|Extensions?|AutoFill|Search|Security|Privacy|Advance|Preferences|Settings)$"
	),
}, { floating = true, workspace = "1" })

rule("float_arc_settings", {
	app_name = exact("Arc"),
	title = regex(
		"^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|curity)|Privacy|Advance|Preferences|Settings)$"
	),
}, { floating = true, workspace = "1" })

rule("float_helium_settings", {
	app_name = exact("Helium"),
	title = regex("^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|security)|Privacy|Advance)$"),
}, { floating = true, workspace = "1" })

-- Floating system and utility windows.
rule("float_1password", { app_name = exact("1Password") }, { floating = true })
rule(
	"float_typora_open",
	{ app_name = exact("Typora"), title = "Открыть" },
	{ floating = true, workspace = "4" }
)
rule("float_finder", { app_bundle = exact("com.apple.finder") }, { floating = true })
rule("float_cisco_secure_client", { app_name = exact("Cisco Secure Client") }, { floating = true })
rule("float_v2raytun", { app_name = exact("v2RayTun") }, { floating = true })
rule("float_raycast", { app_name = exact("Raycast") }, { floating = true })
rule("float_control_center_ru", { app_name = exact("Пункт управления") }, { floating = true })
rule("float_control_center_en", { app_name = exact("Control Center") }, { floating = true })
rule("float_aldente", { app_name = exact("AlDente") }, { floating = true })

-- Workspace 1: browsers and Comet.
rule("ws1_ora", { app_name = exact("Ora") }, { workspace = "1" })
rule("ws1_comet", { app_name = exact("Comet") }, { workspace = "1" })
rule("ws1_thorium", { app_name = exact("Thorium") }, { workspace = "1" })

-- Workspace 2: misc.
rule("ws2_v2box", { app_name = exact("V2BOX") }, { workspace = "2" })
rule("ws2_keet", { app_name = exact("Keet") }, { workspace = "2" })

-- Workspace 3: messaging, mail, music, rooms.
rule("ws3_telegram", { app_name = exact("Telegram") }, { workspace = "3" })
rule("ws3_ayugram", { app_name = exact("AyuGram") }, { workspace = "3" })
rule("ws3_proton_mail", { app_name = exact("Proton Mail") }, { workspace = "3" })
rule("ws3_psst", { app_name = exact("Psst") }, { workspace = "3" })
rule("ws3_x5_rooms", { app_name = exact("X5_Rooms") }, { workspace = "3" })
rule("ws3_spotify", { app_name = exact("Spotify") }, { workspace = "3" })

-- Workspace 4: documents and notes.
rule("ws4_pages", { app_name = exact("Pages") }, { workspace = "4" })
rule("ws4_upnote", { app_name = exact("UpNote") }, { workspace = "4" })

-- Workspace E: system apps.
rule("wse_system_settings_en", { app_name = exact("System Settings") }, { workspace = "E" })
rule("wse_system_preferences_en", { app_name = exact("System Preferences") }, { workspace = "E" })
rule("wse_system_settings_ru", { app_name = exact("Системные настройки") }, { workspace = "E" })
rule("wse_app_store", { app_name = exact("App Store") }, { workspace = "E" })

-- Workspace Q: development tools.
rule("wsq_docker", { app_name = exact("Docker Desktop") }, { workspace = "Q" })
rule("wsq_dbeaver", { app_name = exact("DBeaver Community") }, { workspace = "Q" })
rule("wsq_kubeli", { app_name = exact("Kubeli") }, { workspace = "Q" })
rule("wsq_lens", { app_name = exact("Lens") }, { workspace = "Q" })
rule("wsq_beekeeper", { app_name = exact("Beekeeper Studio") }, { workspace = "Q" })
rule("wsq_postico", { app_name = exact("Postico") }, { workspace = "Q" })

-- Workspace T: Talk and Music.
rule("wst_tolk", { app_name = exact("Толк") }, { workspace = "T" })
rule("wst_music", { app_name = exact("Музыка") }, { workspace = "T" })

-- Workspace W: Preview and Outlook.
rule("wsw_preview_ru", { app_name = exact("Просмотр") }, { workspace = "W" })
rule("wsw_preview_en", { app_name = exact("Preview") }, { workspace = "W" })
rule("wsw_outlook", { app_name = exact("Outlook") }, { workspace = "W" })

-- Workspace D: terminals.
rule("wsd_ghostty", { app_name = exact("Ghostty") }, { workspace = "D" })
rule("wsd_wezterm", { app_name = exact("Wezterm") }, { workspace = "D" })
rule("wsd_alacritty", { app_name = exact("Alacritty") }, { workspace = "D" })
