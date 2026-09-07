return {
	"A7Lavinraj/fyler.nvim",
	dependencies = { "nvim-mini/mini.icons" },
	branch = "stable",
	lazy = false,
	opts = {
		integrations = { icon = "mini_icons" },
		views = {
			finder = {
				win = {
					border = "rounded",
					kinds = {
						float = {
							win_opts = {
								winhighlight = "Normal:NormalFloat,NormalNC:NormalFloat,FloatBorder:FloatBorder",
							},
						},
					},
				},
			},
		},
		hooks = {
			on_rename = function(src_path, destination_path)
				Snacks.rename.on_rename_file(src_path, destination_path)
			end,
		},
	},
}
