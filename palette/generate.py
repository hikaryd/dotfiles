#!/usr/bin/env python3
"""Generate tool-specific color configs from palette/ayu.toml."""

import argparse
import json
import re
from pathlib import Path

import tomllib

ROOT = Path(__file__).resolve().parent.parent
CONFIG = ROOT / "config"


def load_palette(variant: str) -> dict:
    with open(ROOT / "palette" / "ayu.toml", "rb") as f:
        data = tomllib.load(f)
    return data[variant]


def hex_to_rgb(h: str) -> tuple[int, int, int]:
    h = h.lstrip("#")
    return int(h[0:2], 16), int(h[2:4], 16), int(h[4:6], 16)


def hex_to_argb(h: str, alpha: str = "ff") -> str:
    return f"0x{alpha}{h.lstrip('#')}"


# ── Alacritty ────────────────────────────────────────────────────────────────


def gen_alacritty(p: dict, variant: str) -> None:
    path = CONFIG / "alacritty" / "alacritty.toml"
    text = path.read_text()

    colors_block = f"""[colors.primary]
background = "{p["editor"]["bg"]}"
foreground = "{p["editor"]["fg"]}"

[colors.cursor]
cursor = "{p["common"]["accent"]}"
text = "CellBackground"

[colors.selection]
background = "{p["ui"]["selection"]}"
text = "{p["editor"]["fg"]}"

[colors.normal]
black = "{p["terminal"]["black"]}"
red = "{p["terminal"]["red"]}"
green = "{p["terminal"]["green"]}"
yellow = "{p["terminal"]["yellow"]}"
blue = "{p["terminal"]["blue"]}"
magenta = "{p["terminal"]["magenta"]}"
cyan = "{p["terminal"]["cyan"]}"
white = "{p["terminal"]["white"]}"

[colors.bright]
black = "{p["terminal"]["bright_black"]}"
red = "{p["terminal"]["bright_red"]}"
green = "{p["terminal"]["bright_green"]}"
yellow = "{p["terminal"]["bright_yellow"]}"
blue = "{p["terminal"]["bright_blue"]}"
magenta = "{p["terminal"]["bright_magenta"]}"
cyan = "{p["terminal"]["bright_cyan"]}"
white = "{p["terminal"]["bright_white"]}"
"""

    text = re.sub(
        r"^\[colors\.primary\].*?(?=^\[(?!colors(?:\.|\]))|\Z)",
        colors_block,
        text,
        flags=re.DOTALL | re.MULTILINE,
    )
    path.write_text(text)
    print(f"  alacritty: {path}")


# ── Ghostty ──────────────────────────────────────────────────────────────────


def gen_ghostty(p: dict, variant: str) -> None:
    path = CONFIG / "ghostty" / "config"
    text = path.read_text()

    theme_name = "Ayu Mirage" if variant == "mirage" else "Ayu"
    bg = p["editor"]["bg"].lstrip("#")

    text = re.sub(
        r"^theme\s*=\s*.*$", f"theme = {theme_name}", text, flags=re.MULTILINE
    )
    text = re.sub(
        r"^background\s*=\s*.*$", f"background = {bg}", text, flags=re.MULTILINE
    )

    path.write_text(text)
    print(f"  ghostty: {path}")


# ── Wezterm ──────────────────────────────────────────────────────────────────


def gen_wezterm(p: dict, variant: str) -> None:
    path = CONFIG / "wezterm" / "wezterm.lua"
    text = path.read_text()

    bg = p["editor"]["bg"]
    fg = p["editor"]["fg"]
    accent = p["common"]["accent"]
    dim = p["ui"]["fg"]
    t = p["terminal"]

    colors_block = f'''config.colors = {{
\tbackground = "{bg}",
\tforeground = "{fg}",
\tcursor_bg = "{accent}",
\tcursor_fg = "{bg}",
\tcursor_border = "{accent}",
\tselection_bg = "{p["ui"]["selection"]}",
\tselection_fg = "{fg}",
\tsplit = "{p["ui"]["border"]}",
\tansi = {{ "{t["black"]}", "{t["red"]}", "{t["green"]}", "{t["yellow"]}", "{t["blue"]}", "{t["magenta"]}", "{t["cyan"]}", "{t["white"]}" }},
\tbrights = {{ "{t["bright_black"]}", "{t["bright_red"]}", "{t["bright_green"]}", "{t["bright_yellow"]}", "{t["bright_blue"]}", "{t["bright_magenta"]}", "{t["bright_cyan"]}", "{t["bright_white"]}" }},
\ttab_bar = {{
\t\tbackground = "{bg}",
\t\tnew_tab = {{ bg_color = "{bg}", fg_color = "{dim}" }},
\t\tnew_tab_hover = {{ bg_color = "{bg}", fg_color = "{accent}" }},
\t}},
}}'''

    # Replace color_scheme line
    text = re.sub(
        r'^config\.color_scheme\s*=\s*".*"',
        f"-- colors from palette/ayu.toml ({variant})",
        text,
        flags=re.MULTILINE,
    )

    # Replace config.colors block
    text = re.sub(
        r"config\.colors\s*=\s*\{.*?\n\}",
        colors_block,
        text,
        flags=re.DOTALL,
    )

    # Replace local C block
    c_block = f'''local C = {{
\tbg = "{bg}",
\tactive = "{accent}",
\tselection = "{p["ui"]["selection"]}",
\tdim = "{dim}",
}}'''
    text = re.sub(
        r"local C\s*=\s*\{.*?\n\}",
        c_block,
        text,
        flags=re.DOTALL,
    )

    path.write_text(text)
    print(f"  wezterm: {path}")


# ── Starship ─────────────────────────────────────────────────────────────────


def gen_starship(p: dict, variant: str) -> None:
    path = CONFIG / "starship" / "starship.toml"
    text = path.read_text()

    palette_name = f"ayu_{variant}"

    # Replace palette reference
    text = re.sub(
        r'^palette\s*=\s*".*"',
        f'palette = "{palette_name}"',
        text,
        flags=re.MULTILINE,
    )

    # Build new palette — map catppuccin names to ayu values
    e, s, t, u = p["editor"], p["syntax"], p["terminal"], p["ui"]
    palette = f"""# Палитра Ayu {variant.capitalize()}
[palettes.{palette_name}]
color_rosewater = "{s["markup"]}"
color_flamingo = "{s["operator"]}"
color_pink = "{s["markup"]}"
color_mauve = "{s["keyword"]}"
color_red = "{t["red"]}"
color_maroon = "{s["markup"]}"
color_peach = "{s["operator"]}"
color_yellow = "{t["yellow"]}"
color_green = "{t["green"]}"
color_teal = "{t["cyan"]}"
color_sky = "{s["tag"]}"
color_sapphire = "{s["entity"]}"
color_blue = "{s["tag"]}"
color_lavender = "{s["constant"]}"
color_text = "{e["fg"]}"
color_subtext1 = "{e["fg"]}"
color_subtext0 = "{u["fg"]}"
color_overlay2 = "{u["fg"]}"
color_overlay1 = "{u["fg"]}"
color_overlay0 = "{u["fg"]}"
color_surface2 = "{t["bright_black"]}"
color_surface1 = "{t["black"]}"
color_surface0 = "{u["line"]}"
color_base = "{e["bg"]}"
color_mantle = "{e["bg"]}"
color_crust = "{e["bg"]}"
"""

    # Replace palette section
    text = re.sub(
        r"#\s*Палитра.*\n\[palettes\..*?\].*",
        palette,
        text,
        flags=re.DOTALL,
    )

    path.write_text(text)
    print(f"  starship: {path}")


# ── Nushell ──────────────────────────────────────────────────────────────────


def gen_nushell(p: dict, variant: str) -> None:
    path = CONFIG / "nushell" / "ayu.nu"
    e, s, t, u, c = p["editor"], p["syntax"], p["terminal"], p["ui"], p["common"]

    content = f'''# Quiet Ayu {variant} для Nushell
# Сгенерировано из palette/ayu.toml; цвета ввода согласованы с zsh.

let theme = {{
  red: "{t["red"]}"
  green: "{t["green"]}"
  yellow: "{t["yellow"]}"
  blue: "{t["blue"]}"
  magenta: "{t["magenta"]}"
  cyan: "{t["cyan"]}"
  white: "{t["white"]}"
  black: "{t["black"]}"
  text: "{e["fg"]}"
  accent: "{c["accent"]}"
  dim: "{u["fg"]}"
  line: "{u["line"]}"
  selection: "{u["selection"]}"
  warning: "{c["warning"]}"
  bg: "{e["bg"]}"
  tag: "{s["tag"]}"
  func: "{s["func"]}"
  entity: "{s["entity"]}"
  string: "{s["string"]}"
  regexp: "{s["regexp"]}"
  markup: "{s["markup"]}"
  keyword: "{s["keyword"]}"
  special: "{s["special"]}"
  comment: "{s["comment"]}"
  constant: "{s["constant"]}"
  operator: "{s["operator"]}"
  error: "{c["error"]}"
}}

let scheme = {{
  recognized_command: $theme.accent
  unrecognized_command: $theme.text
  constant: $theme.text
  punctuation: $theme.dim
  operator: $theme.dim
  string: $theme.regexp
  virtual_text: $theme.dim
  variable: $theme.text
  filepath: $theme.text
}}

$env.config.color_config = {{
  separator: $theme.dim
  leading_trailing_space_bg: {{ fg: $theme.accent attr: u }}
  header: {{ fg: $theme.text attr: b }}
  row_index: $scheme.virtual_text
  record: $theme.text
  list: $theme.text
  hints: $scheme.virtual_text
  search_result: {{ fg: $theme.accent bg: $theme.selection }}
  shape_closure: $theme.dim
  closure: $theme.dim
  shape_flag: $theme.dim
  shape_matching_brackets: {{ attr: u }}
  shape_garbage: $theme.error
  shape_keyword: $theme.accent
  shape_match_pattern: $scheme.string
  shape_signature: $theme.dim
  shape_table: $scheme.punctuation
  cell-path: $scheme.punctuation
  shape_list: $scheme.punctuation
  shape_record: $scheme.punctuation
  shape_vardecl: $scheme.variable
  shape_variable: $scheme.variable
  empty: {{ attr: n }}
  # Статичные цвета: без радуги и вычислений для каждой ячейки.
  filesize: $theme.text
  duration: $theme.dim
  date: $theme.dim
  shape_external: $scheme.unrecognized_command
  shape_internalcall: $scheme.recognized_command
  shape_external_resolved: $scheme.recognized_command
  shape_block: $scheme.recognized_command
  block: $scheme.recognized_command
  shape_custom: $theme.text
  custom: $theme.text
  background: $theme.bg
  foreground: $theme.text
  cursor: {{ bg: $theme.accent fg: $theme.bg }}
  shape_range: $scheme.operator
  range: $scheme.operator
  shape_pipe: $scheme.operator
  shape_operator: $scheme.operator
  shape_redirection: $scheme.operator
  glob: $scheme.filepath
  shape_directory: $scheme.filepath
  shape_filepath: $scheme.filepath
  shape_glob_interpolation: $scheme.filepath
  shape_globpattern: $scheme.filepath
  shape_int: $scheme.constant
  int: $scheme.constant
  bool: $scheme.constant
  float: $scheme.constant
  nothing: $scheme.constant
  binary: $scheme.constant
  shape_nothing: $scheme.constant
  shape_bool: $scheme.constant
  shape_float: $scheme.constant
  shape_binary: $scheme.constant
  shape_datetime: $scheme.constant
  shape_literal: $scheme.constant
  string: $theme.text
  shape_string: $scheme.string
  shape_string_interpolation: $scheme.string
  shape_raw_string: $scheme.string
  shape_externalarg: $theme.text
}}
$env.config.highlight_resolved_externals = true
$env.config.explore = {{
    status_bar_background: {{ fg: $theme.text, bg: $theme.bg }},
    command_bar_text: {{ fg: $theme.text }},
    highlight: {{ fg: $theme.accent, bg: $theme.selection }},
    status: {{
        error: $theme.error,
        warn: $theme.warning,
        info: $theme.blue,
    }},
    selected_cell: {{ bg: $theme.selection fg: $theme.text }},
}}
'''
    path.write_text(content)
    print(f"  nushell: {path}")


# ── Sketchybar ───────────────────────────────────────────────────────────────


def gen_sketchybar(p: dict, variant: str) -> None:
    path = CONFIG / "sketchybar" / "colors.sh"
    t, e, u, c = p["terminal"], p["editor"], p["ui"], p["common"]

    content = f"""#!/bin/bash
# Ayu {variant} — generated from palette/ayu.toml

export BAR_COLOR={hex_to_argb(e["bg"], "f0")}
export BAR_BORDER_COLOR=0x00000000

export WHITE=0xffffffff
export BLACK=0xff000000
export TRANSPARENT=0x00000000

# Text/icon colors
export ICON_COLOR={hex_to_argb(e["fg"])}
export LABEL_COLOR={hex_to_argb(e["fg"])}

# Subtle backgrounds
export ITEM_BG_COLOR={hex_to_argb(u["selection"], "44")}
export ACCENT_COLOR={hex_to_argb(c["accent"])}
export HIGHLIGHT={hex_to_argb(u["selection"], "66")}

# Semantic colors
export RED={hex_to_argb(c["error"])}
export GREEN={hex_to_argb(t["green"])}
export BLUE={hex_to_argb(t["blue"])}
export YELLOW={hex_to_argb(c["warning"])}
export ORANGE={hex_to_argb(p["syntax"]["operator"])}
export MAGENTA={hex_to_argb(t["magenta"])}
export CYAN={hex_to_argb(t["cyan"])}

# Popup
export POPUP_BACKGROUND_COLOR={hex_to_argb(e["bg"], "e0")}
export POPUP_BORDER_COLOR={hex_to_argb(u["border"], "44")}

export SHADOW_COLOR=$BLACK
"""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)
    print(f"  sketchybar: {path}")


# ── Tmux ─────────────────────────────────────────────────────────────────────


def gen_tmux(p: dict, variant: str) -> None:
    path = CONFIG / "tmux" / "tmux.conf"
    text = path.read_text()

    text = re.sub(
        r"pane-border-style 'fg=#[0-9a-fA-F]+'",
        f"pane-border-style 'fg={p['ui']['border']}'",
        text,
    )
    text = re.sub(
        r"pane-active-border-style 'fg=#[0-9a-fA-F]+'",
        f"pane-active-border-style 'fg={p['common']['accent']}'",
        text,
    )

    path.write_text(text)
    print(f"  tmux: {path}")


# ── Fresh ────────────────────────────────────────────────────────────────────


def gen_fresh(p: dict, variant: str) -> None:
    path = CONFIG / "fresh" / "themes" / f"ayu-{variant}.json"
    e, s, u, c = p["editor"], p["syntax"], p["ui"], p["common"]

    theme = {
        "name": f"ayu-{variant}",
        "editor": {
            "bg": list(hex_to_rgb(e["bg"])),
            "fg": list(hex_to_rgb(e["fg"])),
            "cursor": list(hex_to_rgb(c["accent"])),
            "selection_bg": list(hex_to_rgb(u["selection"])),
            "current_line_bg": list(hex_to_rgb(e["line"])),
            "line_number_fg": list(hex_to_rgb(u["fg"])),
            "line_number_bg": list(hex_to_rgb(e["bg"])),
        },
        "ui": {
            "tab_active_fg": list(hex_to_rgb(c["accent"])),
            "tab_active_bg": list(hex_to_rgb(u["selection"])),
            "tab_inactive_fg": list(hex_to_rgb(u["fg"])),
            "tab_inactive_bg": list(hex_to_rgb(e["bg"])),
            "tab_separator_bg": list(hex_to_rgb(e["bg"])),
            "status_bar_fg": list(hex_to_rgb(e["fg"])),
            "status_bar_bg": list(hex_to_rgb(e["bg"])),
            "prompt_fg": list(hex_to_rgb(c["accent"])),
            "prompt_bg": list(hex_to_rgb(e["bg"])),
            "prompt_selection_fg": list(hex_to_rgb(e["fg"])),
            "prompt_selection_bg": list(hex_to_rgb(u["selection"])),
            "popup_border_fg": list(hex_to_rgb(u["border"])),
            "popup_bg": list(hex_to_rgb(e["bg"])),
            "popup_selection_bg": list(hex_to_rgb(u["selection"])),
            "popup_text_fg": list(hex_to_rgb(e["fg"])),
            "suggestion_bg": list(hex_to_rgb(e["bg"])),
            "suggestion_selected_bg": list(hex_to_rgb(u["selection"])),
            "help_bg": list(hex_to_rgb(e["bg"])),
            "help_fg": list(hex_to_rgb(e["fg"])),
            "help_key_fg": list(hex_to_rgb(s["regexp"])),
            "help_separator_fg": list(hex_to_rgb(u["border"])),
            "help_indicator_fg": list(hex_to_rgb(s["markup"])),
            "help_indicator_bg": list(hex_to_rgb(e["bg"])),
            "split_separator_fg": list(hex_to_rgb(u["border"])),
        },
        "search": {
            "match_bg": list(hex_to_rgb(u["selection"])),
            "match_fg": list(hex_to_rgb(c["accent"])),
        },
        "diagnostic": {
            "error_fg": list(hex_to_rgb(c["error"])),
            "error_bg": list(hex_to_rgb(e["bg"])),
            "warning_fg": list(hex_to_rgb(c["warning"])),
            "warning_bg": list(hex_to_rgb(e["bg"])),
            "info_fg": list(hex_to_rgb(s["tag"])),
            "info_bg": list(hex_to_rgb(e["bg"])),
            "hint_fg": list(hex_to_rgb(u["fg"])),
            "hint_bg": list(hex_to_rgb(e["bg"])),
        },
        "syntax": {
            "keyword": list(hex_to_rgb(s["keyword"])),
            "string": list(hex_to_rgb(s["string"])),
            "comment": list(hex_to_rgb(s["comment"])),
            "function": list(hex_to_rgb(s["func"])),
            "type": list(hex_to_rgb(s["entity"])),
            "variable": list(hex_to_rgb(e["fg"])),
            "constant": list(hex_to_rgb(s["constant"])),
            "operator": list(hex_to_rgb(s["operator"])),
        },
    }

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(theme, indent=2) + "\n")
    print(f"  fresh: {path}")


# ── Attyx ────────────────────────────────────────────────────────────────────


def gen_attyx(p: dict, variant: str) -> None:
    path = CONFIG / "attyx" / "attyx.toml"
    text = path.read_text()

    theme_name = "Ayu Mirage" if variant == "mirage" else "Ayu Dark"
    text = re.sub(
        r'^name\s*=\s*".*"', f'name = "{theme_name}"', text, flags=re.MULTILINE
    )

    path.write_text(text)
    print(f"  attyx: {path}")


# ── Lazygit ──────────────────────────────────────────────────────────────────


def gen_lazygit(p: dict, variant: str) -> None:
    path = CONFIG / "lazygit" / "config.yml"
    text = path.read_text()

    e, u, c, s = p["editor"], p["ui"], p["common"], p["syntax"]

    theme_block = f"""  theme:
    activeBorderColor:
      - "{c["accent"]}"
      - bold
    inactiveBorderColor:
      - "{u["border"]}"
    optionsTextColor:
      - "{s["tag"]}"
    selectedLineBgColor:
      - "{u["selection"]}"
    selectedRangeBgColor:
      - "{u["selection"]}"
    cherryPickedCommitBgColor:
      - "{u["selection"]}"
    cherryPickedCommitFgColor:
      - "{c["accent"]}"
    unstagedChangesColor:
      - "{c["error"]}"
    defaultFgColor:
      - "{e["fg"]}"
    searchingActiveBorderColor:
      - "{c["warning"]}"
"""

    # Меняем только gui.theme: соседние настройки и команды остаются нетронутыми.
    gui = re.search(
        r"^gui:[ \t]*(?:#[^\n]*)?\n(?:^[ \t]+\S[^\n]*(?:\n|$)|^[ \t]*\n)*",
        text,
        re.MULTILINE,
    )
    if gui:
        block = gui.group()
        theme = re.search(
            r"^  theme:[ \t]*\n(?:^[ \t]{4,}\S[^\n]*(?:\n|$)|^[ \t]*\n)*",
            block,
            re.MULTILINE,
        )
        if theme:
            block = block[: theme.start()] + theme_block + block[theme.end() :]
        else:
            header_end = block.index("\n") + 1
            block = block[:header_end] + theme_block + block[header_end:]
        text = text[: gui.start()] + block + text[gui.end() :]
    else:
        text = "gui:\n" + theme_block + "\n" + text

    path.write_text(text)
    print(f"  lazygit: {path}")


# ── Thorium / Chrome theme ───────────────────────────────────────────────────


def gen_thorium(p: dict, variant: str) -> None:
    path = CONFIG / "thorium" / f"ayu-{variant}"
    path.mkdir(parents=True, exist_ok=True)

    e, u, c = p["editor"], p["ui"], p["common"]
    bg = list(hex_to_rgb(e["bg"]))
    fg = list(hex_to_rgb(e["fg"]))
    accent = list(hex_to_rgb(c["accent"]))
    line = list(hex_to_rgb(u["line"]))
    ui_fg = list(hex_to_rgb(u["fg"]))
    toolbar = list(hex_to_rgb(e["line"]))

    manifest = {
        "manifest_version": 3,
        "version": "1.0",
        "name": f"Ayu {variant.capitalize()}",
        "description": f"Ayu {variant} theme — generated from palette/ayu.toml",
        "theme": {
            "colors": {
                "frame": bg,
                "frame_inactive": bg,
                "frame_incognito": bg,
                "frame_incognito_inactive": bg,
                "toolbar": toolbar,
                "tab_text": fg,
                "tab_background_text": ui_fg,
                "tab_background_text_inactive": ui_fg,
                "bookmark_text": fg,
                "ntp_background": bg,
                "ntp_text": fg,
                "ntp_link": accent,
                "ntp_header": line,
                "button_background": [0, 0, 0, 0],
                "omnibox_background": bg,
                "omnibox_text": fg,
            },
            "tints": {
                "frame_inactive": [-1, -1, 0.7],
            },
            "properties": {
                "ntp_background_alignment": "center",
            },
        },
    }

    (path / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"  thorium: {path}/manifest.json")


# ── Yazi (flavor + tmTheme) ──────────────────────────────────────────────────

_MIT = """MIT License

Copyright (c) 2026 ayu-dotfiles

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files, to deal in the software
without restriction, including the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the software, subject to the
above copyright notice and this permission notice being included in all copies.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.
"""

# init.lua template. `__TOKENS__` are replaced with palette colors so the
# yatline status bar stays in sync with the rest of the theme.
_INIT_LUA = """-- Yazi init — generated from palette/ayu.toml by palette/generate.py.
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

    style_a = { fg = "__ACCENT__", bg = "__BG__", bg_mode = { normal = "__BG__", select = "__BG__", un_set = "__BG__" } },
    style_b = { bg = "__UILINE__", fg = "__FG__" },
    style_c = { bg = "reset", fg = "__FG__" },

    permissions_t_fg = "__STRING__",
    permissions_r_fg = "__ACCENT__",
    permissions_w_fg = "__MARKUP__",
    permissions_x_fg = "__REGEXP__",
    permissions_s_fg = "__UIFG__",

    tab_width = 20,
    selected  = { icon = "󰻭", fg = "__ACCENT__" },
    copied    = { icon = "", fg = "__STRING__" },
    cut       = { icon = "", fg = "__MARKUP__" },
    files     = { icon = "", fg = "__TAG__" },
    filtereds = { icon = "", fg = "__CONSTANT__" },
    total     = { icon = "󰮍", fg = "__ACCENT__" },
    success   = { icon = "", fg = "__STRING__" },
    failed    = { icon = "", fg = "__MARKUP__" },

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
"""


def _yz_style(
    fg: str | None = None,
    bg: str | None = None,
    *,
    bold: bool = False,
    italic: bool = False,
    underline: bool = False,
    reverse: bool = False,
) -> str:
    """Render a Yazi inline-table style, e.g. `{ fg = "#e6b450", bold = true }`."""
    parts: list[str] = []
    if fg:
        parts.append(f'fg = "{fg}"')
    if bg:
        parts.append(f'bg = "{bg}"')
    if bold:
        parts.append("bold = true")
    if italic:
        parts.append("italic = true")
    if underline:
        parts.append("underline = true")
    if reverse:
        parts.append("reversed = true")
    return "{ " + ", ".join(parts) + " }" if parts else "{}"


def _tm_scope(
    name: str, scope: str, fg: str | None = None, style: str | None = None
) -> str:
    """Render one tmTheme scope `<dict>` block."""
    body = ""
    if fg:
        body += f"\n\t\t\t\t<key>foreground</key><string>{fg}</string>"
    if style:
        body += f"\n\t\t\t\t<key>fontStyle</key><string>{style}</string>"
    return (
        "\t\t<dict>\n"
        f"\t\t\t<key>name</key><string>{name}</string>\n"
        f"\t\t\t<key>scope</key><string>{scope}</string>\n"
        f"\t\t\t<key>settings</key>\n\t\t\t<dict>{body}\n\t\t\t</dict>\n"
        "\t\t</dict>"
    )


def gen_yazi(p: dict, variant: str) -> None:
    e, s, u, c, vc = (
        p["editor"],
        p["syntax"],
        p["ui"],
        p["common"],
        p["vcs"],
    )

    bg, fg, line, panel = e["bg"], e["fg"], e["line"], e["panel"]
    accent, error = c["accent"], c["error"]
    uifg, uiline = u["fg"], u["line"]
    selection, border, warning = u["selection"], u["border"], c["warning"]
    tag, func, entity = s["tag"], s["func"], s["entity"]
    string, regexp, markup = s["string"], s["regexp"], s["markup"]
    keyword, comment, constant, operator = (
        s["keyword"],
        s["comment"],
        s["constant"],
        s["operator"],
    )
    added, modified, removed = vc["added"], vc["modified"], vc["removed"]

    name = f"ayu-{variant}"
    flavor_dir = CONFIG / "yazi" / "flavors" / f"{name}.yazi"
    flavor_dir.mkdir(parents=True, exist_ok=True)

    st = _yz_style

    def rule(selector: str, inner: str) -> str:
        return f"\t{{ {selector}, {inner} }}"

    rules = ",\n".join(
        [
            rule('mime = "image/*"', f'fg = "{accent}"'),
            rule('mime = "{audio,video}/*"', f'fg = "{constant}"'),
            rule(
                'mime = "application/{zip,rar,7z*,tar,gzip,xz,zstd,bzip*,lzma,compress,archive,cpio,arj,xar,ms-cab*}"',
                f'fg = "{markup}"',
            ),
            rule('mime = "application/{pdf,doc,rtf}"', f'fg = "{tag}"'),
            rule('mime = "vfs/{absent,stale}"', f'fg = "{uifg}"'),
            rule('url = "*", is = "orphan"', f'bg = "{error}"'),
            rule('url = "*", is = "exec"', f'fg = "{string}"'),
            rule('url = "*", is = "dummy"', f'bg = "{error}"'),
            rule('url = "*/", is = "dummy"', f'bg = "{error}"'),
            rule('url = "*/"', f'fg = "{entity}"'),
        ]
    )

    lines = [
        "#:schema https://yazi-rs.github.io/schemas/theme.json",
        f"# Ayu {variant.capitalize()} flavor for Yazi — generated from palette/ayu.toml.",
        "# Edit the palette and run `python3 palette/generate.py`, not this file.",
        "",
        "[mgr]",
        f"cwd = {st(fg=accent)}",
        "",
        f"find_keyword  = {st(fg=accent, bold=True, italic=True, underline=True)}",
        f"find_position = {st(fg=constant, bold=True, italic=True)}",
        "",
        f"symlink_target = {st(fg=uifg, italic=True)}",
        "",
        f"marker_copied   = {st(fg=string, bg=string)}",
        f"marker_cut      = {st(fg=error, bg=error)}",
        f"marker_marked   = {st(fg=tag, bg=tag)}",
        f"marker_selected = {st(fg=accent, bg=accent)}",
        'marker_symbol   = "▏"',
        "",
        f"count_copied   = {st(fg=bg, bg=string)}",
        f"count_cut      = {st(fg=bg, bg=error)}",
        f"count_selected = {st(fg=bg, bg=accent)}",
        "",
        'border_symbol = "│"',
        f"border_style  = {st(fg=border)}",
        "",
        'syntect_theme = ""',
        "",
        "[tabs]",
        f"active   = {st(fg=accent, bg=bg, bold=True)}",
        f"inactive = {st(fg=uifg, bg=line)}",
        'sep_inner = { open = "", close = "" }',
        'sep_outer = { open = "", close = "" }',
        "",
        "[mode]",
        f"normal_main = {st(fg=accent, bg=bg, bold=True)}",
        f"normal_alt  = {st(fg=accent, bg=line)}",
        f"select_main = {st(fg=tag, bg=bg, bold=True)}",
        f"select_alt  = {st(fg=tag, bg=line)}",
        f"unset_main  = {st(fg=markup, bg=bg, bold=True)}",
        f"unset_alt   = {st(fg=markup, bg=line)}",
        "",
        "[indicator]",
        f"parent  = {st(fg=fg, bg=selection)}",
        f"current = {st(fg=fg, bg=selection)}",
        "preview = { underline = true }",
        'padding = { open = "", close = "" }',
        "",
        "[status]",
        "overall   = {}",
        'sep_left  = { open = "", close = "" }',
        'sep_right = { open = "", close = "" }',
        f"perm_sep   = {st(fg=uifg)}",
        f"perm_type  = {st(fg=string)}",
        f"perm_read  = {st(fg=accent)}",
        f"perm_write = {st(fg=markup)}",
        f"perm_exec  = {st(fg=regexp)}",
        f"progress_label  = {st(fg=fg, bold=True)}",
        f"progress_normal = {st(fg=accent, bg=line)}",
        f"progress_error  = {st(fg=bg, bg=error)}",
        "",
        "[which]",
        "cols            = 3",
        f"mask            = {st(bg=panel)}",
        f"cand            = {st(fg=regexp)}",
        f"rest            = {st(fg=uifg)}",
        f"desc            = {st(fg=fg)}",
        'separator       = "  "',
        f"separator_style = {st(fg=uifg)}",
        "",
        "[confirm]",
        f"border  = {st(fg=accent)}",
        f"title   = {st(fg=accent, bold=True)}",
        f"body    = {st(fg=fg)}",
        f"list    = {st(fg=tag)}",
        f"btn_yes = {st(fg=accent, bg=bg, bold=True)}",
        f"btn_no  = {st(fg=fg, bg=line)}",
        'btn_labels = [ "  [Y]es  ", "  (N)o  " ]',
        "",
        "[spot]",
        f"border   = {st(fg=accent)}",
        f"title    = {st(fg=accent, bold=True)}",
        f"tbl_col  = {st(fg=tag)}",
        f"tbl_cell = {st(fg=accent, bg=selection)}",
        "",
        "[notify]",
        f"title_info  = {st(fg=string)}",
        f"title_warn  = {st(fg=warning)}",
        f"title_error = {st(fg=error)}",
        'icon_info  = ""',
        'icon_warn  = ""',
        'icon_error = ""',
        "",
        "[pick]",
        f"border   = {st(fg=accent)}",
        f"active   = {st(fg=accent, bold=True)}",
        f"inactive = {st(fg=fg)}",
        "",
        "[input]",
        f"border   = {st(fg=accent)}",
        f"title    = {st(fg=fg)}",
        f"value    = {st(fg=fg)}",
        f"selected = {st(fg=fg, bg=selection)}",
        "",
        "[cmp]",
        f"border   = {st(fg=accent)}",
        f"active   = {st(fg=fg, bg=selection)}",
        f"inactive = {st(fg=fg)}",
        'icon_file    = ""',
        'icon_folder  = ""',
        'icon_command = ""',
        "",
        "[tasks]",
        f"border  = {st(fg=accent)}",
        f"title   = {st(fg=accent, bold=True)}",
        f"hovered = {st(fg=tag, underline=True)}",
        "",
        "[help]",
        f"on      = {st(fg=regexp)}",
        f"run     = {st(fg=keyword)}",
        f"desc    = {st(fg=fg)}",
        f"hovered = {st(fg=fg, bg=selection, bold=True)}",
        f"footer  = {st(fg=accent, bg=bg)}",
        "",
        "[filetype]",
        "rules = [",
        rules,
        "]",
        "",
        "[git]",
        f"modified  = {st(fg=modified)}",
        f"added     = {st(fg=added)}",
        f"untracked = {st(fg=uifg)}",
        f"ignored   = {st(fg=uifg)}",
        f"deleted   = {st(fg=removed)}",
        f"updated   = {st(fg=accent)}",
        f"unknown   = {st(fg=uifg)}",
        f"clean     = {st(fg=string)}",
        'modified_sign  = "•"',
        'added_sign     = "✚"',
        'untracked_sign = "?"',
        'ignored_sign   = ""',
        'deleted_sign   = "✗"',
        'updated_sign   = "↑"',
        'unknown_sign   = ""',
        'clean_sign     = ""',
        "",
    ]
    flavor_text = "\n".join(lines) + "\n"
    (flavor_dir / "flavor.toml").write_text(flavor_text)

    # tmTheme for code-preview syntax highlighting (syntect).
    scopes = [
        ("Comment", "comment, punctuation.definition.comment", comment, "italic"),
        ("String", "string", string, None),
        ("Regexp", "string.regexp, constant.character.escape", regexp, None),
        (
            "Constant",
            "constant.numeric, constant.language, constant.character, constant.other",
            constant,
            None,
        ),
        (
            "Keyword",
            "keyword, keyword.control, storage, storage.type, storage.modifier",
            keyword,
            None,
        ),
        (
            "Operator",
            "keyword.operator, punctuation.separator.operator",
            operator,
            None,
        ),
        (
            "Function",
            "entity.name.function, support.function, meta.function-call, variable.function",
            func,
            None,
        ),
        (
            "Type",
            "entity.name.type, entity.name.class, support.type, support.class, entity.other.inherited-class",
            entity,
            None,
        ),
        ("Tag", "entity.name.tag, meta.tag", tag, None),
        ("Attribute", "entity.other.attribute-name", func, None),
        ("Variable", "variable, variable.parameter, variable.other", fg, None),
        ("Language variable", "variable.language", markup, "italic"),
        ("Support constant", "support.constant, constant.other.color", constant, None),
        ("Punctuation", "punctuation, meta.brace, meta.delimiter", fg, None),
        ("Heading", "markup.heading, entity.name.section", func, "bold"),
        ("Bold", "markup.bold", None, "bold"),
        ("Italic", "markup.italic", None, "italic"),
        ("Link", "markup.underline.link, string.other.link", tag, "underline"),
        ("Raw", "markup.raw, markup.inline.raw", string, None),
        ("Quote", "markup.quote", comment, "italic"),
        ("List", "markup.list, punctuation.definition.list", keyword, None),
        ("Inserted", "markup.inserted", added, None),
        ("Deleted", "markup.deleted", removed, None),
        ("Changed", "markup.changed", modified, None),
        ("Invalid", "invalid, invalid.illegal", error, None),
    ]
    tm = [
        '<?xml version="1.0" encoding="UTF-8"?>',
        (
            '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" '
            '"http://www.apple.com/DTDs/PropertyList-1.0.dtd">'
        ),
        '<plist version="1.0">',
        "<dict>",
        "\t<key>name</key>",
        f"\t<string>Ayu {variant.capitalize()}</string>",
        "\t<key>settings</key>",
        "\t<array>",
        "\t\t<dict>",
        "\t\t\t<key>settings</key>",
        "\t\t\t<dict>",
        f"\t\t\t\t<key>background</key><string>{bg}</string>",
        f"\t\t\t\t<key>foreground</key><string>{fg}</string>",
        f"\t\t\t\t<key>caret</key><string>{accent}</string>",
        f"\t\t\t\t<key>lineHighlight</key><string>{line}</string>",
        f"\t\t\t\t<key>selection</key><string>{selection}</string>",
        f"\t\t\t\t<key>invisibles</key><string>{uifg}</string>",
        "\t\t\t</dict>",
        "\t\t</dict>",
    ]
    tm += [_tm_scope(*sc) for sc in scopes]
    tm += [
        "\t</array>",
        "\t<key>uuid</key>",
        f"\t<string>ayu-{variant}-yazi</string>",
        "</dict>",
        "</plist>",
    ]
    (flavor_dir / "tmtheme.xml").write_text("\n".join(tm) + "\n")

    (flavor_dir / "LICENSE").write_text(_MIT)
    (flavor_dir / "LICENSE-tmtheme").write_text(_MIT)

    (CONFIG / "yazi" / "theme.toml").write_text(
        "#:schema https://yazi-rs.github.io/schemas/theme.json\n"
        "# Generated from palette/ayu.toml.\n"
        "# Цвета заданы и здесь: tmux может не ответить на запрос цветовой схемы.\n"
        "# Оба режима используют выбранный вариант; меняйте его через generator.\n\n"
        "[flavor]\n"
        f'dark  = "{name}"\n'
        f'light = "{name}"\n\n' + flavor_text.rstrip() + "\n"
    )

    # init.lua — plugin setup + ayu-colored yatline status bar.
    init_subs = {
        "__BG__": bg,
        "__FG__": fg,
        "__ACCENT__": accent,
        "__TAG__": tag,
        "__MARKUP__": markup,
        "__STRING__": string,
        "__REGEXP__": regexp,
        "__UIFG__": uifg,
        "__UILINE__": uiline,
        "__CONSTANT__": constant,
    }
    init_lua = _INIT_LUA
    for token, color in init_subs.items():
        init_lua = init_lua.replace(token, color)
    (CONFIG / "yazi" / "init.lua").write_text(init_lua)

    print(f"  yazi: {flavor_dir}/flavor.toml (+ tmtheme.xml, theme.toml, init.lua)")


# ── Main ─────────────────────────────────────────────────────────────────────

GENERATORS = {
    "alacritty": gen_alacritty,
    "ghostty": gen_ghostty,
    "wezterm": gen_wezterm,
    "starship": gen_starship,
    "nushell": gen_nushell,
    "sketchybar": gen_sketchybar,
    "tmux": gen_tmux,
    "fresh": gen_fresh,
    "attyx": gen_attyx,
    "lazygit": gen_lazygit,
    "thorium": gen_thorium,
    "yazi": gen_yazi,
}


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate configs from ayu palette")
    parser.add_argument("--variant", choices=["dark", "mirage"], default="dark")
    parser.add_argument(
        "--tool", choices=list(GENERATORS.keys()), help="Generate for one tool only"
    )
    parser.add_argument(
        "--dry-run", action="store_true", help="Print what would be done"
    )
    args = parser.parse_args()

    palette = load_palette(args.variant)
    targets = [args.tool] if args.tool else list(GENERATORS.keys())

    print(f"Generating ayu {args.variant} configs:")
    for name in targets:
        if args.dry_run:
            print(f"  [dry-run] {name}")
        else:
            GENERATORS[name](palette, args.variant)

    print("Done.")


if __name__ == "__main__":
    main()
