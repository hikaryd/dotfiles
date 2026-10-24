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

    style_a = { fg = "#73d0ff", bg = "#1f2430", bg_mode = { normal = "#1f2430", select = "#1f2430", un_set = "#1f2430" } },
    style_b = { bg = "#171b24", fg = "#cccac2" },
    style_c = { bg = "reset", fg = "#cccac2" },

    permissions_t_fg = "#d5ff80",
    permissions_r_fg = "#73d0ff",
    permissions_w_fg = "#f28779",
    permissions_x_fg = "#95e6cb",
    permissions_s_fg = "#707a8c",

    tab_width = 20,
    selected  = { icon = "󰻭", fg = "#73d0ff" },
    copied    = { icon = "", fg = "#d5ff80" },
    cut       = { icon = "", fg = "#f28779" },
    files     = { icon = "", fg = "#5ccfe6" },
    filtereds = { icon = "", fg = "#dfbfff" },
    total     = { icon = "󰮍", fg = "#73d0ff" },
    success   = { icon = "", fg = "#d5ff80" },
    failed    = { icon = "", fg = "#f28779" },

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
