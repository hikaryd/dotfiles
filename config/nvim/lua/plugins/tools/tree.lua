return {
	{
		"nvim-neo-tree/neo-tree.nvim",
		branch = "v3.x",
		dependencies = {
			"nvim-lua/plenary.nvim",
			"MunifTanjim/nui.nvim",
			"nvim-mini/mini.icons",
		},
		-- Fyler — основной explorer; Neo-tree сохраняем доступным по команде,
		-- но не оплачиваем его и три зависимости при каждом запуске Neovim.
		cmd = "Neotree",
	},
}
