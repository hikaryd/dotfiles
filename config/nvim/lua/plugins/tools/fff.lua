return {
	"dmtrKovalenko/fff.nvim",
	build = function()
		require("fff.download").download_or_build_binary()
	end,
	lazy = false,
	opts = {
		lazy_sync = true,
		title = "FFFiles",
		max_results = 100,
		layout = {
			height = 0.8,
			width = 0.87,
			prompt_position = "top",
			preview_position = "right",
			preview_size = 0.55,
			show_scrollbar = true,
			path_shorten_strategy = "middle_number",
		},
		preview = {
			enabled = true,
			line_numbers = false,
			wrap_lines = false,
			filetypes = {
				markdown = { wrap_lines = true },
				svg = { wrap_lines = true },
				text = { wrap_lines = true },
			},
		},
		keymaps = {
			close = "<Esc>",
			select = "<CR>",
			select_split = "<C-s>",
			select_vsplit = "<C-v>",
			select_tab = "<C-t>",
			move_up = { "<Up>", "<C-p>" },
			move_down = { "<Down>", "<C-n>" },
			preview_scroll_up = "<C-u>",
			preview_scroll_down = "<C-d>",
			cycle_grep_modes = "<S-Tab>",
			toggle_select = "<Tab>",
			send_to_quickfix = "<C-q>",
		},
		git = {
			status_text_color = false,
		},
		grep = {
			smart_case = true,
			modes = { "plain", "regex", "fuzzy" },
			trim_whitespace = false,
		},
	},
	keys = {
		{
			"<leader><space>",
			function()
				require("fff").find_files()
			end,
			desc = "FFF find files",
		},
		{
			"<leader>/",
			function()
				require("fff").live_grep()
			end,
			desc = "FFF live grep",
		},
		{
			"<leader>fo",
			function()
				require("fff").find_files()
			end,
			desc = "FFF frecency files",
		},
		{
			"<leader>fw",
			function()
				require("fff").live_grep({ query = vim.fn.expand("<cword>") })
			end,
			desc = "FFF grep word under cursor",
		},
		{
			"<leader>fz",
			function()
				require("fff").live_grep({ grep = { modes = { "fuzzy", "plain" } } })
			end,
			desc = "FFF fuzzy grep",
		},
		{
			"<leader>fS",
			function()
				require("fff").scan_files()
			end,
			desc = "FFF rescan files",
		},
	},
}
