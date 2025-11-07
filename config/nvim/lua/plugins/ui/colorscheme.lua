return {
	{
		"catppuccin/nvim",
		name = "catppuccin",
		priority = 1000,
		lazy = false,
		config = function()
			require("catppuccin").setup({
				flavour = "mocha",
				transparent_background = true,
				term_colors = true,
				integrations = {
					aerial = true,
					alpha = true,
					gitsigns = true,
					diffview = true,
					blink_cmp = true,
					illuminate = true,
					indent_blankline = { enabled = true },
					lsp_trouble = true,
					mason = true,
					mini = {
						enabled = true,
						indentscope_color = "",
					},
					native_lsp = {
						enabled = true,
						underlines = {
							errors = { "underline" },
							hints = { "underline" },
							warnings = { "underline" },
							information = { "underline" },
						},
					},
					navic = { enabled = true, custom_bg = "NONE" },
					neotest = true,
					noice = true,
					snacks = true,
					harpoon = false,
					notify = true,
					neotree = true,
					semantic_tokens = true,
					telescope = true,
					treesitter = true,
					which_key = true,
				},
				styles = {
					comments = {},
					conditionals = { "bold" },
					loops = { "bold" },
					functions = { "bold" },
					keywords = {},
					strings = {},
					variables = {},
					numbers = { "bold" },
					booleans = { "bold" },
					properties = {},
					types = { "bold" },
					operators = { "bold" },
				},
				custom_highlights = function(colors)
					return {
						Comment = { fg = colors.overlay1 },
						LineNr = { fg = colors.overlay2 },
						CursorLine = { bg = colors.surface0 },
						CursorLineNr = { fg = colors.lavender, bold = true },
						Search = { bg = colors.surface1, fg = colors.text, bold = true },

						-- Make floats and Telescope transparent
						NormalFloat = { bg = "NONE" },
						FloatBorder = { bg = "NONE", fg = colors.overlay0 },
						Pmenu = { bg = "NONE" },
						PmenuSel = { bg = colors.surface1 },

						-- Link Telescope to float styles for consistent palette
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
					}
				end,
			})
			vim.cmd.colorscheme("catppuccin")
		end,
	},
	-- {
	--   'rose-pine/neovim',
	--   name = 'rose-pine',
	--   priority = 1000,
	--   lazy = false,
	--   config = function()
	--     require('rose-pine').setup {
	--       variant = 'moon', -- auto, main, moon, or dawn
	--       dark_variant = 'moon', -- main, moon, or dawn
	--       dim_inactive_windows = false,
	--       extend_background_behind_borders = true,
	--
	--       enable = {
	--         terminal = true,
	--         legacy_highlights = false,
	--         migrations = true,
	--       },
	--
	--       styles = {
	--         bold = true,
	--         italic = true,
	--         transparency = true,
	--       },
	--     }
	--
	--     -- vim.cmd 'colorscheme rose-pine-main'
	--     -- vim.cmd 'colorscheme rose-pine-moon'
	--     -- vim.cmd("colorscheme rose-pine-dawn")
	--     vim.cmd 'colorscheme rose-pine'
	--   end,
	-- },
}
