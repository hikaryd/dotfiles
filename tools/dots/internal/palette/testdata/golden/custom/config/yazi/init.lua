-- Yazi init — generated from palette/ayu.toml by palette/generate.py.
-- Do not edit by hand. Plugins are installed via `ya pkg` / dotbot (steps/tools.yml).

-- Starship prompt as the file-list header (matches the shell prompt).
pcall(function() require("starship"):setup() end)

-- Rounded borders around the parent / current / preview panes.
pcall(function() require("full-border"):setup({ type = ui.Border.ROUNDED }) end)

-- Git status signs in the file list.
pcall(function() require("git"):setup() end)

-- Restore files deleted by Yazi (keymap: u / U).
pcall(function() require("restore"):setup() end)

-- Ayu-themed, lualine-style status bar. The header is left to starship.
pcall(function()
  require("yatline"):setup({
    section_separator = { open = "", close = "" },
    part_separator    = { open = "", close = "" },
    inverse_separator = { open = "", close = "" },

    style_a = { fg = "#887058", bg = "#132231", bg_mode = { normal = "#132231", select = "#132231", un_set = "#132231" } },
    style_b = { bg = "#252e37", fg = "#162432" },
    style_c = { bg = "reset", fg = "#162432" },

    permissions_t_fg = "#373a3d",
    permissions_r_fg = "#887058",
    permissions_w_fg = "#3d3e3f",
    permissions_x_fg = "#3a3c3e",
    permissions_s_fg = "#1f2a35",

    tab_width = 20,
    selected  = { icon = "󰻭", fg = "#887058" },
    copied    = { icon = "", fg = "#373a3d" },
    cut       = { icon = "", fg = "#3d3e3f" },
    files     = { icon = "", fg = "#2e343a" },
    filtereds = { icon = "", fg = "#494643" },
    total     = { icon = "󰮍", fg = "#887058" },
    success   = { icon = "", fg = "#373a3d" },
    failed    = { icon = "", fg = "#3d3e3f" },

    show_background     = false,
    display_header_line = false,
    display_status_line = true,

    status_line = {
      left = {
        section_a = { { type = "string", name = "tab_mode" } },
        section_b = { { type = "string", name = "hovered_size" } },
        section_c = {
          { type = "string",   name = "hovered_path" },
          { type = "coloreds", name = "count" },
        },
      },
      right = {
        section_a = { { type = "string", name = "cursor_position" } },
        section_b = { { type = "string", name = "cursor_percentage" } },
        section_c = {
          { type = "string",   name = "hovered_file_extension", params = { true } },
          { type = "coloreds", name = "permissions" },
        },
      },
    },
  })
end)
