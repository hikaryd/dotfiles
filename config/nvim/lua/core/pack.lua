-- Минимальный нативный плагин-менеджер для Neovim
-- Использует ~/.local/share/nvim/site/pack/dots/{start,opt}
local M = {}

local data = vim.fn.stdpath('data')
local pack_dir = data .. '/site/pack/dots/start'

local function ensure_dir()
  vim.fn.mkdir(pack_dir, 'p')
end

local function repo_name(spec)
  if spec.name then return spec.name end
  local src = spec[1] or ''
  local n = src:match('([^/]+)$') or src
  return (n:gsub('%.git$', ''))
end

local function plugin_path(name)
  return pack_dir .. '/' .. name
end

local function url_of(spec)
  if spec.url then return spec.url end
  return 'https://github.com/' .. spec[1] .. '.git'
end

local function clone(spec)
  local name = repo_name(spec)
  local dest = plugin_path(name)
  if (vim.uv or vim.loop).fs_stat(dest) then return false end
  vim.notify('[pack] installing ' .. name, vim.log.levels.INFO)
  local out = vim.fn.system({
    'git', 'clone', '--filter=blob:none', '--depth=1', url_of(spec), dest,
  })
  if vim.v.shell_error ~= 0 then
    vim.notify('[pack] failed to clone ' .. name .. ': ' .. out, vim.log.levels.ERROR)
    return false
  end
  return true
end

local function normalize(spec)
  if type(spec) == 'string' then return { spec } end
  return spec
end

-- Module может вернуть либо одну спеку (mod[1] = "user/repo"),
-- либо список спек, либо список import-таблиц. Нормализуем к списку.
local function as_list(mod)
  if type(mod) ~= 'table' then return {} end
  if type(mod[1]) == 'string' then return { mod } end
  return mod
end

local function import_module(mod_path)
  -- Сначала пробуем как одиночный модуль (lua/<path>.lua или lua/<path>/init.lua)
  local ok, mod = pcall(require, mod_path)
  if ok then return as_list(mod) end
  -- Иначе сканируем каталог lua/<mod_path>/*.lua
  local rel = mod_path:gsub('%.', '/')
  local files = vim.api.nvim_get_runtime_file('lua/' .. rel .. '/*.lua', true)
  local specs = {}
  for _, file in ipairs(files) do
    local name = file:match('([^/]+)%.lua$')
    if name and name ~= 'init' then
      local m_ok, m = pcall(require, mod_path .. '.' .. name)
      if m_ok then
        for _, s in ipairs(as_list(m)) do
          table.insert(specs, s)
        end
      end
    end
  end
  return specs
end

local function is_enabled(spec)
  if spec.enabled == nil then return true end
  if type(spec.enabled) == 'function' then
    local ok, v = pcall(spec.enabled)
    return ok and v
  end
  return spec.enabled
end

local function flatten(specs, out, seen)
  out = out or {}
  seen = seen or {}
  for _, raw in ipairs(specs) do
    local spec = normalize(raw)
    if spec.import then
      local mod = import_module(spec.import)
      flatten(mod, out, seen)
    elseif spec[1] and is_enabled(spec) then
      if spec.dependencies then
        for _, dep in ipairs(spec.dependencies) do
          local d = normalize(dep)
          if d[1] and is_enabled(d) and not seen[d[1]] then
            seen[d[1]] = true
            table.insert(out, d)
          end
        end
      end
      if not seen[spec[1]] then
        seen[spec[1]] = true
        table.insert(out, spec)
      end
    end
  end
  return out
end

local function run_build(cmd, plugin_dir)
  if type(cmd) == 'function' then
    pcall(cmd)
  elseif type(cmd) == 'string' then
    if cmd:sub(1, 1) == ':' then
      pcall(vim.cmd, cmd:sub(2))
    else
      vim.notify('[pack] running build: ' .. cmd .. ' in ' .. plugin_dir, vim.log.levels.INFO)
      local out = vim.fn.system('cd ' .. vim.fn.shellescape(plugin_dir) .. ' && ' .. cmd)
      if vim.v.shell_error ~= 0 then
        vim.notify('[pack] build failed: ' .. out, vim.log.levels.ERROR)
      end
    end
  end
end

local function module_name(spec)
  if spec.main then return spec.main end
  local name = repo_name(spec)
  name = name:gsub('%.nvim$', ''):gsub('%.lua$', '')
  return name
end

local function configure(spec, freshly_installed)
  if spec.cond ~= nil then
    local ok = type(spec.cond) == 'function' and spec.cond() or spec.cond
    if not ok then return end
  end

  local name = repo_name(spec)
  pcall(vim.cmd, 'packadd! ' .. name)

  if freshly_installed then
    run_build(spec.build or spec.make, plugin_path(name))
  end

  if type(spec.init) == 'function' then
    pcall(spec.init, spec)
  end

  if type(spec.keys) == 'table' then
    for _, k in ipairs(spec.keys) do
      if type(k) == 'table' and k[1] then
        local lhs = k[1]
        local rhs = k[2]
        local mode = k.mode or 'n'
        local opts = {
          desc = k.desc,
          silent = k.silent ~= false,
          noremap = k.noremap ~= false,
          expr = k.expr,
          nowait = k.nowait,
        }
        if rhs ~= nil then
          pcall(vim.keymap.set, mode, lhs, rhs, opts)
        end
      end
    end
  end

  local config = spec.config or spec.on
  local has_opts = spec.opts ~= nil

  if type(config) == 'function' then
    local opts = type(spec.opts) == 'function' and spec.opts() or spec.opts or {}
    pcall(config, spec, opts)
  elseif has_opts or config == true then
    local opts = type(spec.opts) == 'function' and spec.opts() or spec.opts or {}
    local mod_name = module_name(spec)
    local ok, mod = pcall(require, mod_name)
    if ok and type(mod.setup) == 'function' then
      pcall(mod.setup, opts)
    end
  end
end

function M.setup(specs)
  ensure_dir()
  local plugins = flatten(specs)
  M._plugins = plugins

  -- Сначала устанавливаем все недостающие, потом подгружаем
  local fresh = {}
  for _, spec in ipairs(plugins) do
    fresh[repo_name(spec)] = clone(spec)
  end

  -- Сортируем по priority (выше = раньше)
  table.sort(plugins, function(a, b)
    return (a.priority or 50) > (b.priority or 50)
  end)

  for _, spec in ipairs(plugins) do
    pcall(configure, spec, fresh[repo_name(spec)])
  end
end

function M.update()
  local count = 0
  for _, spec in ipairs(M._plugins or {}) do
    if not spec.pin then
      local name = repo_name(spec)
      local dest = plugin_path(name)
      if (vim.uv or vim.loop).fs_stat(dest) then
        vim.fn.system({ 'git', '-C', dest, 'pull', '--ff-only' })
        count = count + 1
      end
    end
  end
  vim.notify('[pack] updated ' .. count .. ' plugins', vim.log.levels.INFO)
end

function M.list()
  local lines = {}
  for _, spec in ipairs(M._plugins or {}) do
    table.insert(lines, '  ' .. repo_name(spec) .. '  (' .. (spec[1] or '?') .. ')')
  end
  vim.notify(table.concat(lines, '\n'), vim.log.levels.INFO)
end

vim.api.nvim_create_user_command('PackUpdate', M.update, { desc = 'Update all plugins' })
vim.api.nvim_create_user_command('PackList', M.list, { desc = 'List installed plugins' })

return M
