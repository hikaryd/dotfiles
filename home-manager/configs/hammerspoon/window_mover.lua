-- === window_mover.lua ===
-- Основано на коде из PaperWM.spoon

local WindowMover = {}
WindowMover.log = hs.logger.new("WindowMover", "info")

-- --- Вспомогательные функции ---

-- Модули Hammerspoon
local Axuielement = hs.axuielement
local Event = hs.eventtap.event
local EventTypes = hs.eventtap.event.types
local Geometry = hs.geometry
local Spaces = hs.spaces
local Timer = hs.timer
local Screen = hs.screen

-- Блокирующая пауза
local function wait(seconds)
	local start = Timer.secondsSinceEpoch()
	while Timer.secondsSinceEpoch() - start < seconds do
	end
end

-- Функции для эмуляции мыши
local function mouseMove(position)
	Event.newMouseEvent(EventTypes.mouseMoved, position):post()
end
local function mouseDown(position)
	Event.newMouseEvent(EventTypes.leftMouseDown, position):post()
end
local function mouseUp(position)
	Event.newMouseEvent(EventTypes.leftMouseUp, position):post()
end
local function mouseClick(position)
	mouseDown(position)
	mouseUp(position)
end

-- Эмуляция перетаскивания
local function mouseDrag(start_pos, end_pos)
	mouseMove(start_pos)
	mouseDown(start_pos)
	wait(0.1)
	mouseMove(end_pos)
	Event.newMouseEvent(EventTypes.leftMouseDragged, end_pos):post()
	mouseUp(end_pos)
end

local function getMissionControlGroup()
	local dock = Axuielement.applicationElement("com.apple.dock")
	if not dock then
		return nil, "Не удалось получить доступ к Dock.app"
	end

	for _, element in ipairs(dock:children()) do
		if element:attributeValue("AXIdentifier") == "mc" then
			return element
		end
	end
	return nil, "Mission Control не открыт"
end

local function getMissionControlWindows()
	local mc_group, err = getMissionControlGroup()
	if err or not mc_group then
		return nil, err
	end

	local windows = {}
	local displays = mc_group:children({ AXIdentifier = "mc.display" })
	for _, display in ipairs(displays) do
		local windows_container = display:children({ AXIdentifier = "mc.windows" })[1]
		if windows_container then
			for _, mc_window in ipairs(windows_container:children() or {}) do
				table.insert(windows, mc_window)
			end
		end
	end
	return windows
end

local function getMissionControlSpaces()
	local mc_group, err = getMissionControlGroup()
	if err or not mc_group then
		return nil, err
	end

	local spaces = {}
	local displays = mc_group:children({ AXIdentifier = "mc.display" })
	for _, display in ipairs(displays) do
		local spaces_container = display:children({ AXIdentifier = "mc.spaces" })[1]
		if spaces_container then
			local spaces_list = spaces_container:children({ AXIdentifier = "mc.spaces.list" })[1]
			if spaces_list then
				for _, mc_space in ipairs(spaces_list:children() or {}) do
					table.insert(spaces, mc_space)
				end
			end
		end
	end
	return spaces
end

---
-- Получает системный ID воркспейса по его порядковому номеру.
-- @param index (number) - Порядковый номер воркспейса (1, 2, 3...).
-- @return (number|nil) - Системный ID или nil, если не найден.
---
function WindowMover:getSpaceID(index)
	local layout = Spaces.allSpaces()
	for _, screen in ipairs(Screen.allScreens()) do
		local screen_uuid = screen:getUUID()
		local num_spaces = #layout[screen_uuid]
		if num_spaces >= index then
			return layout[screen_uuid][index]
		end
		index = index - num_spaces
	end
	return nil
end

---
-- Получает визуальный индекс воркспейса в Mission Control по его системному ID.
-- @param space_id (number) - Системный ID воркспейса.
-- @return (number|nil) - Индекс или nil.
---
function WindowMover:getSpaceIndex(space_id)
	local layout = Spaces.allSpaces()
	local index = 0
	for _, screen in ipairs(Screen.allScreens()) do
		local screen_uuid = screen:getUUID()
		for i, space in ipairs(layout[screen_uuid]) do
			if space == space_id then
				return index + i
			end
		end
		index = index + #layout[screen_uuid]
	end
	return nil
end

---
-- Основная функция. Перемещает указанное окно на указанный воркспейс.
-- @param window (hs.window) - Объект окна для перемещения.
-- @param target_space_id (number) - Системный ID целевого воркспейса.
-- @return (boolean, string|nil) - true в случае успеха, false и сообщение об ошибке в случае неудачи.
---
function WindowMover:moveToSpace(window, target_space_id)
	local title = window:title()
	if not title or #title == 0 then
		title = window:application():title()
	end
	if not title or #title == 0 then
		return false, "У окна нет заголовка"
	end

	local target_space_index = self:getSpaceIndex(target_space_id)
	if not target_space_index then
		return false, "Не удалось найти индекс целевого воркспейса"
	end

	self.log.i(
		"Перемещение окна '"
			.. title
			.. "' на воркспейс с индексом "
			.. target_space_index
	)

	-- 1. Открываем Mission Control и двигаем мышь, чтобы показать все воркспейсы
	Spaces.openMissionControl()
	wait(0.2)
	mouseMove({ x = 10, y = 10 })
	wait(hs.spaces.MCwaitTime or 0.5)

	-- 2. Находим миниатюру нашего окна в Mission Control
	local mc_windows, err = getMissionControlWindows()
	if err or not mc_windows then
		Spaces.closeMissionControl()
		return false, "Не удалось получить окна из Mission Control: " .. tostring(err)
	end

	local start_position
	for _, mc_win in ipairs(mc_windows) do
		if mc_win:attributeValue("AXWindow") and mc_win:attributeValue("AXWindow"):id() == window:id() then
			start_position = Geometry(mc_win:attributeValue("AXFrame")).center
			break
		end
	end

	if not start_position then
		Spaces.closeMissionControl()
		return false, "Не удалось найти миниатюру окна в Mission Control"
	end

	-- 3. Находим миниатюру целевого воркспейса
	local mc_spaces, err = getMissionControlSpaces()
	if err or not mc_spaces then
		Spaces.closeMissionControl()
		return false,
			"Не удалось получить воркспейсы из Mission Control: " .. tostring(err)
	end

	local target_space_element = mc_spaces[target_space_index]
	if not target_space_element then
		Spaces.closeMissionControl()
		return false,
			"Не удалось найти миниатюру воркспейса с индексом "
				.. target_space_index
	end

	local end_position = Geometry(target_space_element:attributeValue("AXFrame")).center
	self.log.i("Перетаскивание с " .. tostring(start_position) .. " на " .. tostring(end_position))

	-- 4. Перетаскиваем окно на воркспейс и кликаем, чтобы переключиться на него
	mouseDrag(start_position, end_position)
	wait(hs.spaces.MCwaitTime or 0.5)
	mouseClick(end_position) -- Кликаем, чтобы выйти из Mission Control и перейти на нужный воркспейс

	return true
end

return WindowMover
