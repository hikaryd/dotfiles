return {
	"folke/snacks.nvim",
	priority = 10000,
	lazy = false,
	opts = {
		input = { enabled = true },
		bigfile = { enabled = true },
		notifier = {
			enabled = true,
			timeout = 3000,
		},
		quickfile = { enabled = true },
		statuscolumn = { enabled = true },
		words = { enabled = true },
		styles = {
			input = { border = "rounded" },
			notification = {
				border = "rounded",
				wo = { wrap = true },
			},
		},
		lazygit = {
			enabled = true,
		},
		animate = {
			enabled = true,
			duration = 20,
			easing = "linear",
			fps = 60,
		},
		indent = {
			enabled = true,
		},
		toggle = {
			enabled = true,
		},
		image = { enabled = false },
		picker = {
			sources = {
				explorer = {
					auto_close = true,
					layout = { preset = "vscode" },
				},
			},
		},
		dashboard = { enabled = false },
	},
	keys = {
		{
			"<leader>d",
			function()
				Snacks.picker.diagnostics()
			end,
		},
		{
			"<leader>e",
			function()
				Snacks.explorer.reveal()
			end,
		},
		{
			"gd",
			function()
				Snacks.picker.lsp_definitions()
			end,
		},
		{
			"gr",
			function()
				Snacks.picker.lsp_references()
			end,
			nowait = true,
		},
	},
	init = function()
		vim.api.nvim_create_autocmd("User", {
			pattern = "VeryLazy",
			callback = function()
				Snacks.toggle.option("spell", { name = "Spelling" }):map("<leader>us")
			end,
		})
	end,
}
