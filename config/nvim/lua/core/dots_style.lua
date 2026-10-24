-- Профиль меняет только оформление, не содержимое и не набор buffers.
local M = {}
local saved, windows = {}, {}
local current
local modes = { glass = true, focus = true, presentation = true }

local function style()
	local root = vim.env.XDG_CONFIG_HOME
	if not root or root == "" then
		root = vim.env.HOME .. "/.config"
	end
	local file = io.open(root .. "/dots/style", "r")
	if not file then
		return "glass"
	end
	local value = file:read("*l")
	file:close()
	return modes[value] and value or "glass"
end

local function set(name, value)
	saved[name] = { before = vim.o[name], applied = value }
	vim.o[name] = value
end

local function restore()
	for name, value in pairs(saved) do
		if name == "snacks_animate" then
			if vim.g.snacks_animate == value.applied then
				vim.g.snacks_animate = value.before
			end
		elseif vim.o[name] == value.applied then
			vim.o[name] = value.before
		end
	end
	for win, value in pairs(windows) do
		if vim.api.nvim_win_is_valid(win) and vim.wo[win].winbar == "" then
			vim.wo[win].winbar = value
		end
	end
	saved, windows = {}, {}
end

function M.refresh()
	local next_style = style()
	if next_style ~= current then
		restore()
		current = next_style
		if current ~= "glass" then
			saved.snacks_animate = { before = vim.g.snacks_animate, applied = false }
			vim.g.snacks_animate = false
			set("smoothscroll", false)
			set("guicursor", (vim.o.guicursor:gsub("blinkon%d+", "blinkon0")))
		end
		if current == "presentation" then
			set("laststatus", 0)
			set("showtabline", 0)
			set("title", false)
		end
	end
	if current == "presentation" then
		for _, win in ipairs(vim.api.nvim_list_wins()) do
			if windows[win] == nil or vim.wo[win].winbar ~= "" then
				-- Сохраняем последнее значение producer, а не устаревший путь.
				windows[win] = vim.wo[win].winbar
				vim.wo[win].winbar = ""
			end
		end
	end
end

function M.setup()
	local group = vim.api.nvim_create_augroup("DotsStyle", { clear = true })
	vim.api.nvim_create_autocmd({ "VimEnter", "FocusGained", "WinEnter", "BufWinEnter", "LspAttach" }, {
		group = group,
		callback = function()
			vim.schedule(M.refresh)
		end,
	})
	local function after_render()
		if current == "presentation" then
			vim.schedule(M.refresh)
		end
	end
	-- OptionSet не вызывается из non-nested autocmd плагина.
	vim.api.nvim_create_autocmd({ "CursorMoved", "CursorMovedI" }, {
		group = group,
		callback = after_render,
	})
	vim.api.nvim_create_autocmd("User", {
		group = group,
		pattern = "SagaSymbolUpdate",
		callback = after_render,
	})
	-- Lspsaga пишет winbar после CursorMoved и асинхронного SagaSymbolUpdate.
	-- Не отключаем его autocmds: откладываем очистку до окончания записи option.
	vim.api.nvim_create_autocmd("OptionSet", {
		group = group,
		pattern = "winbar",
		callback = after_render,
	})
	vim.schedule(M.refresh)
end

return M
