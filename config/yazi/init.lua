-- Yazi init — generated from palette/ayu.toml by dots-tools palette.
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

    style_a = { fg = "#ffc799", bg = "#101010", bg_mode = { normal = "#101010", select = "#101010", un_set = "#101010" } },
    style_b = { bg = "#232323", fg = "#ffffff" },
    style_c = { bg = "reset", fg = "#ffffff" },

    permissions_t_fg = "#99ffe4",
    permissions_r_fg = "#ffc799",
    permissions_w_fg = "#ff8080",
    permissions_x_fg = "#50b5aa",
    permissions_s_fg = "#a0a0a0",

    tab_width = 20,
    selected  = { icon = "󰻭", fg = "#ffc799" },
    copied    = { icon = "", fg = "#99ffe4" },
    cut       = { icon = "", fg = "#ff8080" },
    files     = { icon = "", fg = "#ffc799" },
    filtereds = { icon = "", fg = "#ffc799" },
    total     = { icon = "󰮍", fg = "#ffc799" },
    success   = { icon = "", fg = "#99ffe4" },
    failed    = { icon = "", fg = "#ff8080" },

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
