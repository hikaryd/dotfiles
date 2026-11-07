return {
	{
		"Shatur/neovim-ayu",
		priority = 1000,
		lazy = false,
		config = function()
			-- Keep transparent editor and opaque popups.
			local colors = {
				bg = "#101010",
				fg = "#ffffff",
				muted = "#a0a0a0",
				border = "#282828",
				selection = "#232323",
				accent = "#ffc799",
				error = "#ff8080",
				warn = "#ffc799",
				success = "#99ffe4",
				strings = "#99ffe4",
			}
			local palette = require("ayu.colors")
			local generate = palette.generate
			palette.generate = function(...)
				generate(...)
				for key, value in pairs({
					bg = colors.bg, fg = colors.fg, ui = colors.muted, accent = colors.accent,
					tag = colors.accent, func = colors.accent, entity = colors.accent,
					string = colors.strings, regexp = "#50b5aa", markup = colors.error,
					keyword = "#b0b0b0", special = colors.accent, comment = "#8b8b8b",
					constant = colors.accent, operator = "#b0b0b0", error = colors.error,
					lsp_parameter = colors.fg, lsp_inlay_hint = colors.muted,
					line = "#1c1c1c", panel_bg = "#181818", panel_shadow = colors.bg,
					panel_border = colors.border, gutter_normal = colors.border, gutter_active = colors.muted,
					selection_bg = colors.selection, selection_inactive = colors.selection, selection_border = colors.border,
					guide_active = colors.muted, guide_normal = colors.border,
					vcs_added = colors.success, vcs_modified = colors.accent, vcs_removed = colors.error,
					vcs_added_bg = colors.selection, vcs_removed_bg = colors.selection,
					fg_idle = colors.muted, warning = colors.warn,
				}) do palette[key] = value end
				vim.g.terminal_color_background = colors.bg
				vim.g.terminal_color_foreground = colors.fg
				for i, color in ipairs({ "#181818", "#ff8080", "#50b5aa", "#ffc799", "#b0b0b0", "#ffc799", "#50b5aa", "#dadada", "#505050", "#ff8080", "#99ffe4", "#ffcfa8", "#b0b0b0", "#ffcfa8", "#99ffe4", "#ffffff" }) do
					vim.g["terminal_color_" .. (i - 1)] = color
				end
			end

			require("ayu").setup({
				mirage = false,
				terminal = false,
				overrides = {
					Normal = { fg = colors.fg, bg = "NONE" },
					NormalNC = { fg = colors.fg, bg = "NONE" },
					SignColumn = { bg = "NONE" },
					FoldColumn = { bg = "NONE" },

					Comment = { fg = colors.muted },
					String = { fg = colors.strings },
					["@string"] = { link = "String" },
					Function = { fg = colors.accent },
					["@function"] = { link = "Function" },
					["@function.call"] = { link = "Function" },
					Directory = { fg = colors.accent },
					Title = { fg = colors.accent, bold = true },
					Number = { fg = colors.warn },
					Keyword = { fg = "#b0b0b0" },
					LineNr = { fg = colors.muted },
					CursorLine = { bg = colors.bg },
					CursorLineNr = { fg = colors.accent, bold = true },
					Search = { bg = colors.selection, fg = colors.fg, bold = true },

					NormalFloat = { fg = colors.fg, bg = colors.bg },
					FloatBorder = { bg = colors.bg, fg = colors.border },
					Pmenu = { bg = colors.bg, fg = colors.fg },
					PmenuSel = { bg = colors.selection, fg = colors.fg, reverse = false },
					Visual = { bg = colors.selection },
					WinSeparator = { fg = colors.border, bg = "NONE" },
					FloatTitle = { fg = colors.accent, bg = colors.bg, bold = true },
					DiagnosticError = { fg = colors.error },
					DiagnosticWarn = { fg = colors.warn },
					DiagnosticInfo = { fg = colors.accent },
					DiagnosticHint = { fg = colors.strings },
					DiagnosticOk = { fg = colors.success },
					MiniIconsBlue = { fg = colors.accent },
					MiniIconsCyan = { fg = colors.strings },
					MiniIconsGreen = { fg = colors.success },
					MiniIconsGrey = { fg = colors.muted },
					MiniIconsOrange = { fg = colors.warn },
					MiniIconsPurple = { fg = colors.accent },
					MiniIconsRed = { fg = colors.error },
					MiniIconsYellow = { fg = colors.warn },
					FylerNormal = { link = "Normal" },
					FylerNormalNC = { link = "NormalNC" },
					FylerBorder = { link = "FloatBorder" },
					NeoTreeNormal = { link = "Normal" },
					NeoTreeNormalNC = { link = "NormalNC" },
					NeoTreeFloatNormal = { link = "NormalFloat" },
					NeoTreeFloatBorder = { link = "FloatBorder" },
					SnacksInputNormal = { link = "NormalFloat" },
					SnacksInputBorder = { link = "FloatBorder" },
					SnacksInputTitle = { link = "FloatTitle" },
					SnacksPicker = { link = "NormalFloat" },
					SnacksPickerBorder = { link = "FloatBorder" },
					SnacksPickerTitle = { link = "FloatTitle" },

					TelescopeNormal = { link = "NormalFloat" },
					TelescopeBorder = { link = "FloatBorder" },
					TelescopePromptNormal = { link = "NormalFloat" },
					TelescopeResultsNormal = { link = "NormalFloat" },
					TelescopePreviewNormal = { link = "NormalFloat" },
					TelescopePromptBorder = { link = "FloatBorder" },
					TelescopeResultsBorder = { link = "FloatBorder" },
					TelescopePreviewBorder = { link = "FloatBorder" },
					TelescopePromptTitle = { link = "Title" },
					TelescopeResultsTitle = { link = "Title" },
					TelescopePreviewTitle = { link = "Title" },
				},
			})

			vim.cmd.colorscheme("ayu")
		end,
	},
}
