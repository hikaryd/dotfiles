return {
	"nvim-mini/mini.icons",
	lazy = false,
	priority = 900,
	config = function()
		local icons = require("mini.icons")
		icons.setup()
		-- Один источник иконок, включая потребителей API nvim-web-devicons.
		icons.mock_nvim_web_devicons()
	end,
}
