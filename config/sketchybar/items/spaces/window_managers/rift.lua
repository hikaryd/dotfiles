-- rift Window Manager integration for SketchyBar
-- Credits: https://github.com/falleco/dotfiles/blob/main/sketchybar
--
-- ⚙️  НАСТРОЙКА СОБЫТИЙ (опционально, для уменьшения polling):
--
-- Для автоматического обновления при изменении воркспейсов выполните команды:
--
--   rift-cli subscribe cli \
--     --event workspace_changed \
--     --command "sketchybar --trigger rift_workspace_changed"
--
--   rift-cli subscribe cli \
--     --event windows_changed \
--     --command "sketchybar --trigger rift_windows_changed"
--
-- Это позволит обновлять UI моментально при изменениях вместо polling каждые 5 секунд.
--
---@diagnostic disable: need-check-nil
local app_icons = require("helpers.spaces_util.app_icons")
local sbar_utils = require("helpers.spaces_util.sbar_util")

-- helpers  ---------------------------------------------------------

-- Создаем space item для rift без привязки к macOS Spaces
local function add_rift_space_item(space_name, idx)
  -- Для rift используем оригинальные имена воркспейсов (1, 2, 3, e, q, t, w, d)
  local space_label = space_name

  local space = SBAR.add("item", "space." .. space_name, {
    icon = {
      font = { family = FONT.nerd_font },
      string = space_label,
      padding_left = SPACE_ITEM_PADDING,
      padding_right = 8,
      color = COLORS.text,
      highlight_color = COLORS.mauve,
    },
    label = {
      padding_right = SPACE_ITEM_PADDING,
      color = COLORS.grey,
      highlight_color = COLORS.lavender,
      font = "sketchybar-app-font:Regular:16.0",
      y_offset = -1,
    },
    padding_right = 1,
    padding_left = 1,
    background = {
      color = COLORS.base,
      border_width = 2,
      height = 26,
      border_color = COLORS.mantle,
    },
  })

  local space_bracket = SBAR.add("bracket", { space.name }, {
    drawing = false,
    background = {
      color = COLORS.transparent,
      border_color = COLORS.surface1,
      height = 28,
      border_width = 2,
    },
  })

  SBAR.add("item", "space.padding." .. idx, {
    width = GROUP_PADDINGS,
  })

  return { space = space, bracket = space_bracket }
end

-- Парсим JSON-вывод rift-cli query workspaces используя jq
local function get_workspaces()
  local handle = io.popen([[
    rift-cli query workspaces | jq -c '.[] | {
      id: .id,
      index: .index,
      name: .name,
      is_active: .is_active,
      window_count: .window_count,
      windows: [.windows[] | {bundle_id: .bundle_id, title: .title}]
    }'
  ]])

  local workspaces = {}
  if handle then
    for line in handle:lines() do
      if line ~= "" then
        -- Простой Lua JSON парсинг для упрощенного формата
        local ws = {}
        ws.id = line:match('"id":"([^"]+)"')
        ws.index = tonumber(line:match('"index":(%d+)'))
        ws.name = line:match('"name":"([^"]+)"')
        ws.is_active = line:match('"is_active":(true)') ~= nil
        ws.window_count = tonumber(line:match('"window_count":(%d+)'))

        -- Парсим окна
        ws.windows = {}
        local windows_str = line:match('"windows":%[(.-)%]')
        if windows_str and windows_str ~= "" then
          for win_block in windows_str:gmatch("%b{}") do
            local bundle_id = win_block:match('"bundle_id":"([^"]*)"')
            local title = win_block:match('"title":"([^"]*)"')
            table.insert(ws.windows, {
              bundle_id = bundle_id,
              title = title,
            })
          end
        end

        table.insert(workspaces, ws)
      end
    end
    handle:close()
  end

  table.sort(workspaces, function(a, b)
    return a.index < b.index
  end)

  return workspaces
end

local rift_workspaces = get_workspaces()

local function get_current_workspace()
  for _, ws in ipairs(rift_workspaces) do
    if ws.is_active then
      return ws
    end
  end
end

local initial_current_workspace = get_current_workspace()

-- Window manager  --------------------------------------------------

local Window_Manager = {
  events = {
    window_change = "rift_windows_changed",
    focus_change = "rift_workspace_changed",
  },
  spaces = {}, -- сюда кладём item-ы sketchybar (item, а не только item.space)
  observer = nil,
}

-- Регистрируем кастомные события rift в sketchybar
SBAR.exec("sketchybar --add event rift_workspace_changed")
SBAR.exec("sketchybar --add event rift_windows_changed")

function Window_Manager:init()
  -- Обновим состояние на момент инициализации
  rift_workspaces = get_workspaces()

  for i, ws in ipairs(rift_workspaces) do
    local selected = ws.is_active

    -- Используем кастомную функцию для rift без привязки к macOS Spaces
    local item = add_rift_space_item(ws.name, i)

    -- Сохраняем целиком item, чтобы потом можно было и label менять, и подсветку обновлять
    self.spaces[i] = item

    sbar_utils.highlight_focused_space(item, selected)

    -- Клик по иконке воркспейса
    item.space:subscribe("mouse.clicked", function(env)
      self:perform_switch_desktop(env.BUTTON, ws.name, ws.id)
    end)
  end

  -- Первый проход по иконкам
  self:update_space_label()
end

function Window_Manager:start_watcher()
  -- Сохраняем ссылку на self для использования в callback
  local manager = self

  -- Глобальный watcher для событий rift
  -- События rift глобальные, поэтому создаем один watcher для всех воркспейсов
  local event_watcher = SBAR.add("item", {
    drawing = false,
    updates = true,
  })

  -- Подписка на события изменения воркспейса
  event_watcher:subscribe("rift_workspace_changed", function(ev)
    LOG.log("rift_workspace_changed event received!")
    manager:update_space_label()
  end)

  -- Подписка на события изменения окон
  event_watcher:subscribe("rift_windows_changed", function(ev)
    LOG.log("rift_windows_changed event received!")
    manager:update_space_label()
  end)

  -- Fallback поллинг на случай, если события не настроены
  -- Можно отключить, если события работают стабильно
  local polling_watcher = SBAR.add("item", {
    drawing = false,
    updates = true,
    update_freq = 5,
  })

  polling_watcher:subscribe("routine", function(_)
    manager:update_space_label()
  end)
end

--- @param button string  нажатая кнопка мыши ("left", "right", "other")
--- @param name   string  rift workspace name ("1", "q", "d", ...)
--- @param id     string  rift internal id ("VirtualWorkspaceId(9v1)", ...)
function Window_Manager:perform_switch_desktop(button, name, id)
  if button == "left" then
    -- Переключаемся на воркспейс по имени
    local cmd = string.format("rift-cli execute workspace switch %s", name)
    SBAR.exec(cmd)
  elseif button == "right" then
    -- Можно реализовать перемещение окна на воркспейс
    -- local cmd = string.format("rift-cli execute workspace move-window %s", name)
    -- SBAR.exec(cmd)
  elseif button == "other" then
    -- Средняя кнопка - можно добавить другой функционал
  end
end

function Window_Manager:update_space_label()
  LOG.log("update_space_label called")

  -- Читаем актуальное состояние из rift
  local workspaces = get_workspaces()
  rift_workspaces = workspaces

  LOG.log(string.format("Found %d workspaces", #workspaces))

  for i, ws in ipairs(workspaces) do
    LOG.log(string.format("Workspace %d: name=%s, active=%s", i, ws.name, tostring(ws.is_active)))

    local icon_line = ""
    local no_app = true

    -- Собираем список иконок по bundle_id (и, на всякий случай, title)
    for _, win in ipairs(ws.windows or {}) do
      no_app = false
      local bundle = win.bundle_id or ""
      local title = win.title or ""
      local lookup = app_icons[bundle] or app_icons[title]
      local icon = lookup or app_icons["default"]
      icon_line = icon_line .. " " .. icon
    end

    if no_app then
      icon_line = " —"
    end

    local item = self.spaces[i]
    if item and item.space then
      SBAR.animate("tanh", 10, function()
        item.space:set({
          label = icon_line,
        })
      end)

      -- Одновременно обновляем подсветку активного воркспейса
      LOG.log(string.format("Setting highlight for workspace %d to %s", i, tostring(ws.is_active)))
      sbar_utils.highlight_focused_space(item, ws.is_active)
    else
      LOG.log(string.format("WARNING: No item found for workspace %d", i))
    end
  end
end

return Window_Manager
