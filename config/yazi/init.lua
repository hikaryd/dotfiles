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

    style_a = { fg = "#0d1017", bg = "#e6b450", bg_mode = { normal = "#e6b450", select = "#39bae6", un_set = "#f07178" } },
    style_b = { bg = "#1b1f29", fg = "#bfbdb6" },
    style_c = { bg = "reset", fg = "#bfbdb6" },

    permissions_t_fg = "#aad94c",
    permissions_r_fg = "#e6b450",
    permissions_w_fg = "#f07178",
    permissions_x_fg = "#95e6cb",
    permissions_s_fg = "#555e73",

    tab_width = 20,
    selected  = { icon = "󰻭", fg = "#e6b450" },
    copied    = { icon = "", fg = "#aad94c" },
    cut       = { icon = "", fg = "#f07178" },
    files     = { icon = "", fg = "#39bae6" },
    filtereds = { icon = "", fg = "#d2a6ff" },
    total     = { icon = "󰮍", fg = "#e6b450" },
    success   = { icon = "", fg = "#aad94c" },
    failed    = { icon = "", fg = "#f07178" },

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
