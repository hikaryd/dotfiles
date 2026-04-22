return {
	{
		"Shatur/neovim-ayu",
		priority = 1000,
		lazy = false,
		config = function()
			local colors = require("ayu.colors")
			colors.generate(false) -- false = dark, true = mirage

			require("ayu").setup({
				mirage = false,
				terminal = true,
				overrides = {
					Normal = { bg = "NONE" },
					NormalNC = { bg = "NONE" },
					SignColumn = { bg = "NONE" },
					FoldColumn = { bg = "NONE" },

					Comment = { fg = colors.comment },
					LineNr = { fg = colors.guide_active },
					CursorLine = { bg = colors.line },
					CursorLineNr = { fg = colors.accent, bold = true },
					Search = { bg = colors.selection_bg, fg = colors.fg, bold = true },

					NormalFloat = { bg = "NONE" },
					FloatBorder = { bg = "NONE", fg = colors.guide_active },
					Pmenu = { bg = "NONE" },
					PmenuSel = { bg = colors.selection_bg },

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
