-- Минимальный нативный плагин-менеджер для Neovim.
-- Плагины хранятся в pack/dots/opt и загружаются только по заявленным триггерам.
local M = {}

local uv = vim.uv or vim.loop
local data = vim.fn.stdpath("data")
local pack_root = data .. "/site/pack/dots"
local start_dir = pack_root .. "/start"
local opt_dir = pack_root .. "/opt"

local function ensure_dirs()
	vim.fn.mkdir(start_dir, "p")
	vim.fn.mkdir(opt_dir, "p")
end

local function repo_name(spec)
	if spec.name then
		return spec.name
	end
	local src = spec[1] or ""
	return ((src:match("([^/]+)$") or src):gsub("%.git$", ""))
end

local function plugin_path(name)
	return opt_dir .. "/" .. name
end

local function normalize(spec)
	return type(spec) == "string" and { spec } or spec
end

local function as_list(mod)
	if type(mod) ~= "table" then
		return {}
	end
	return type(mod[1]) == "string" and { mod } or mod
end

local function import_module(mod_path)
	local ok, mod = pcall(require, mod_path)
	if ok then
		return as_list(mod)
	end

	local files = vim.api.nvim_get_runtime_file("lua/" .. mod_path:gsub("%.", "/") .. "/*.lua", true)
	local specs = {}
	table.sort(files)
	for _, file in ipairs(files) do
		local name = file:match("([^/]+)%.lua$")
		if name and name ~= "init" then
			local loaded, value = pcall(require, mod_path .. "." .. name)
			if loaded then
				vim.list_extend(specs, as_list(value))
			end
		end
	end
	return specs
end

local function is_enabled(spec)
	if spec.enabled == nil then
		return true
	end
	if type(spec.enabled) ~= "function" then
		return spec.enabled
	end
	local ok, value = pcall(spec.enabled)
	return ok and value
end

local function collect(raw_specs)
	local specs, by_name = {}, {}
	local function visit(items, top_level)
		for _, raw in ipairs(items) do
			local spec = normalize(raw)
			if spec.import then
				visit(import_module(spec.import), true)
			elseif spec[1] and is_enabled(spec) then
				local name = repo_name(spec)
				local current = by_name[name]
				if not current then
					current = spec
					current._dependency_names = {}
					current._top_level = top_level
					by_name[name] = current
					table.insert(specs, current)
				elseif top_level then
					current._top_level = true
				end
				for _, raw_dep in ipairs(spec.dependencies or {}) do
					local dep = normalize(raw_dep)
					if dep[1] and is_enabled(dep) then
						table.insert(current._dependency_names, repo_name(dep))
						visit({ dep }, false)
					end
				end
			end
		end
	end
	visit(raw_specs, true)
	return specs, by_name
end

-- Старый менеджер складывал всё в start, из-за чего Neovim неизбежно грузил
-- каждый plugin/*.lua. Однократно переносим каталоги в opt без удаления данных.
local function migrate_legacy_start_plugins()
	for name, kind in vim.fs.dir(start_dir) do
		if kind == "directory" then
			local source = start_dir .. "/" .. name
			local destination = plugin_path(name)
			if not uv.fs_stat(destination) then
				local ok, err = uv.fs_rename(source, destination)
				if not ok then
					vim.notify(("[pack] failed to migrate %s: %s"):format(name, err), vim.log.levels.WARN)
				end
			end
		end
	end
end

local function clone(spec)
	local name = repo_name(spec)
	local destination = plugin_path(name)
	if uv.fs_stat(destination) then
		return false
	end
	vim.notify("[pack] installing " .. name, vim.log.levels.INFO)
	local output = vim.fn.system({
		"git",
		"clone",
		"--filter=blob:none",
		"--depth=1",
		spec.url or ("https://github.com/" .. spec[1] .. ".git"),
		destination,
	})
	if vim.v.shell_error ~= 0 then
		vim.notify("[pack] failed to clone " .. name .. ": " .. output, vim.log.levels.ERROR)
		return false
	end
	return true
end

local function run_build(command, directory)
	if type(command) == "function" then
		pcall(command)
	elseif type(command) == "string" then
		if command:sub(1, 1) == ":" then
			pcall(vim.cmd, command:sub(2))
		else
			local output = vim.fn.system("cd " .. vim.fn.shellescape(directory) .. " && " .. command)
			if vim.v.shell_error ~= 0 then
				vim.notify("[pack] build failed: " .. output, vim.log.levels.ERROR)
			end
		end
	end
end

local function module_name(spec)
	if spec.main then
		return spec.main
	end
	return repo_name(spec):gsub("%.nvim$", ""):gsub("%.lua$", "")
end

local function configure(spec)
	if spec.cond ~= nil then
		local ok = type(spec.cond) == "function" and spec.cond() or spec.cond
		if not ok then
			return
		end
	end
	local config = spec.config or spec.on
	local opts = spec.opts or {}
	if type(opts) == "function" then
		local defaults = {}
		opts = opts(spec, defaults) or defaults
	end
	for _, key in ipairs(spec.keys or {}) do
		if type(key) == "table" and key[1] and key[2] ~= nil then
			vim.keymap.set(key.mode or "n", key[1], key[2], {
				desc = key.desc,
				silent = key.silent ~= false,
				noremap = key.noremap ~= false,
				expr = key.expr,
				nowait = key.nowait,
			})
		end
	end
	if type(config) == "function" then
		pcall(config, spec, opts)
	elseif spec.opts ~= nil or config == true then
		local ok, mod = pcall(require, module_name(spec))
		if ok and type(mod.setup) == "function" then
			pcall(mod.setup, opts)
		end
	end
end

local function is_lazy(spec)
	if spec.lazy == false then
		return false
	end
	return spec.event ~= nil or spec.ft ~= nil or spec.cmd ~= nil or spec.keys ~= nil or not spec._top_level
end

local function values(value)
	if value == nil then
		return {}
	end
	return type(value) == "table" and value or { value }
end

function M.setup(raw_specs)
	ensure_dirs()
	migrate_legacy_start_plugins()

	local specs, by_name = collect(raw_specs)
	M._plugins, M._by_name, M._loaded = specs, by_name, {}
	local fresh = {}
	for _, spec in ipairs(specs) do
		fresh[repo_name(spec)] = clone(spec)
	end

	local function load(name)
		if M._loaded[name] then
			return
		end
		local spec = by_name[name]
		if not spec then
			return
		end
		M._loaded[name] = true
		for _, dependency in ipairs(spec._dependency_names) do
			load(dependency)
		end
		vim.cmd("packadd " .. vim.fn.fnameescape(name))
		if fresh[name] then
			run_build(spec.build or spec.make, plugin_path(name))
		end
		configure(spec)
	end
	M.load = load

	table.sort(specs, function(a, b)
		return (a.priority or 50) > (b.priority or 50)
	end)
	local group = vim.api.nvim_create_augroup("DotsPluginLoader", { clear = true })

	for _, spec in ipairs(specs) do
		local name = repo_name(spec)
		if type(spec.init) == "function" then
			pcall(spec.init, spec)
		end
		if not is_lazy(spec) then
			load(name)
		else
			for _, event in ipairs(values(spec.event)) do
				if event == "VeryLazy" then
					vim.api.nvim_create_autocmd("User", {
						group = group,
						pattern = "VeryLazy",
						once = true,
						callback = function()
							load(name)
						end,
					})
				else
					vim.api.nvim_create_autocmd(event, {
						group = group,
						once = true,
						callback = function()
							load(name)
						end,
					})
				end
			end
			if spec.ft then
				vim.api.nvim_create_autocmd("FileType", {
					group = group,
					pattern = values(spec.ft),
					once = true,
					callback = function()
						load(name)
					end,
				})
			end
			for _, command in ipairs(values(spec.cmd)) do
				vim.api.nvim_create_user_command(command, function(ctx)
					pcall(vim.api.nvim_del_user_command, command)
					load(name)
					local range = ctx.range > 0 and (ctx.line1 .. "," .. ctx.line2) or ""
					vim.cmd(ctx.mods .. " " .. range .. command .. (ctx.bang and "!" or "") .. " " .. ctx.args)
				end, { bang = true, nargs = "*", range = true })
			end
		end
	end

	vim.schedule(function()
		vim.api.nvim_exec_autocmds("User", { pattern = "VeryLazy", modeline = false })
	end)
end

function M.update()
	local count = 0
	for _, spec in ipairs(M._plugins or {}) do
		if not spec.pin and uv.fs_stat(plugin_path(repo_name(spec))) then
			vim.fn.system({ "git", "-C", plugin_path(repo_name(spec)), "pull", "--ff-only" })
			count = count + 1
		end
	end
	vim.notify("[pack] updated " .. count .. " plugins", vim.log.levels.INFO)
end

function M.list()
	local lines = {}
	for _, spec in ipairs(M._plugins or {}) do
		local name = repo_name(spec)
		table.insert(lines, ("  %s  [%s]"):format(name, M._loaded[name] and "loaded" or "lazy"))
	end
	vim.notify(table.concat(lines, "\n"), vim.log.levels.INFO)
end

vim.api.nvim_create_user_command("PackUpdate", M.update, { desc = "Update all plugins" })
vim.api.nvim_create_user_command("PackList", M.list, { desc = "List installed plugins" })

return M
