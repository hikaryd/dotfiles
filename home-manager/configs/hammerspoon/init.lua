-- =============================================================================
-- Hammerspoon Configuratiion
-- =============================================================================

-- Глобальный логгер
local log = hs.logger.new("MainConfig", "info")

-- Горячая клавиша для быстрой перезагрузки конфига
hs.hotkey.bind({ "cmd", "alt", "ctrl" }, "r", function()
	hs.reload()
	hs.alert.show("Hammerspoon config reloaded!")
end)

-- Вспомогательная функция для вызова yabai
function yabai(args)
	return hs.task.new("/opt/homebrew/bin/yabai", nil, args)
end

function runYabai(args)
	yabai(args):start()
end

-- Фокус окон (HJKL)
hs.hotkey.bind({ "alt" }, "h", function()
	runYabai({ "-m", "window", "--focus", "west" })
end)
hs.hotkey.bind({ "alt" }, "j", function()
	runYabai({ "-m", "window", "--focus", "south" })
end)
hs.hotkey.bind({ "alt" }, "k", function()
	runYabai({ "-m", "window", "--focus", "north" })
end)
hs.hotkey.bind({ "alt" }, "l", function()
	runYabai({ "-m", "window", "--focus", "east" })
end)

-- Перемещение окон (Swap)
hs.hotkey.bind({ "shift", "alt" }, "h", function()
	runYabai({ "-m", "window", "--swap", "west" })
end)
hs.hotkey.bind({ "shift", "alt" }, "j", function()
	runYabai({ "-m", "window", "--swap", "south" })
end)
hs.hotkey.bind({ "shift", "alt" }, "k", function()
	runYabai({ "-m", "window", "--swap", "north" })
end)
hs.hotkey.bind({ "shift", "alt" }, "l", function()
	runYabai({ "-m", "window", "--swap", "east" })
end)

-- Полноэкранный режим
hs.hotkey.bind({ "alt", "shift" }, "f", function()
	runYabai({ "-m", "window", "--toggle", "zoom-parent" })
end)

-- Балансировка размеров окон в воркспейсе
hs.hotkey.bind({ "alt", "shift" }, "0", function()
	runYabai({ "-m", "space", "--balance" })
end)

-- Перемещение окна на другой дисплей и фокус на нем
hs.hotkey.bind({ "ctrl", "alt" }, "h", function()
	runYabai({ "-m", "window", "--display", "west" })
	runYabai({ "-m", "display", "--focus", "west" })
end)
hs.hotkey.bind({ "ctrl", "alt" }, "l", function()
	runYabai({ "-m", "window", "--display", "east" })
	runYabai({ "-m", "display", "--focus", "east" })
end)

-- Фокус на другом дисплее
hs.hotkey.bind({ "cmd", "shift" }, "l", function()
	runYabai({ "-m", "display", "--focus", "next" })
end)
hs.hotkey.bind({ "cmd", "shift" }, "h", function()
	runYabai({ "-m", "display", "--focus", "prev" })
end)

-- Переключение между floating и tiling
hs.hotkey.bind({ "cmd", "shift" }, "f", function()
	runYabai({ "-m", "window", "--toggle", "float" })
end)

-- Закрытие окна
hs.hotkey.bind({ "alt" }, "q", function()
	runYabai({ "-m", "window", "--close" })
end)

-- ====================
-- УПРАВЛЕНИЕ ВОРКСПЕЙСАМИ (SPACES)
-- ====================

-- Подключаем наш новый модуль
-- Convert seconds to microseconds
local timeUnit = 1000 * 1000
-- delay: in seconds
function asyncLeftClick(point, delay, onFinished)
	local module = hs.eventtap
	module.event.newMouseEvent(module.event.types["leftMouseDown"], point):post()

	hs.timer.doAfter(delay, function()
		module.event.newMouseEvent(module.event.types["leftMouseUp"], point):post()
		if onFinished then
			onFinished()
		end
	end)
end

-- Move window to space
function moveWindowToSpace(window, spaceNumber)
	log.i("Moving window " .. window:title() .. " to space " .. spaceNumber)
	local prevCursorPoint = hs.mouse.absolutePosition()
	local winFrame = window:frame()
	local point = hs.geometry(winFrame.x + 5, winFrame.y + 15)
	asyncLeftClick(point, 1, function()
		-- Restore cursor position
		hs.mouse.absolutePosition(prevCursorPoint)
	end)
	-- Switch to target space with Mission Control shortcuts
	if spaceNumber < 10 then
		hs.eventtap.keyStroke({ "alt" }, tostring(spaceNumber), 0.2 * timeUnit)
	else
		hs.eventtap.keyStroke({ "alt", "ctrl" }, tostring(spaceNumber - 10), 0.2 * timeUnit)
	end
end

-- Function to move focused window to a specific space
function moveFocusedWindowToSpace(spaceNumber)
	local spaceName = "Desktop " .. spaceNumber
	log.i("Attempting to move window to " .. spaceName)
	local focusedWindow = hs.window.focusedWindow()
	if focusedWindow then
		moveWindowToSpace(focusedWindow, spaceNumber)
	else
		log.w("No focused window")
		hs.alert.show("No focused window")
	end
end

-- Bind keys cmd + shift + 0-9
for i = 1, 9 do
	hs.hotkey.bind({ "alt", "shift" }, tostring(i), function()
		moveFocusedWindowToSpace(i)
	end)
end

-- --- 2. Переключение на предыдущий воркспейс ---

local currentSpace = nil
local previousSpace = nil

function getCurrentSpaceNumber()
	local screen = hs.screen.mainScreen()
	local activeSpace = hs.spaces.activeSpaceOnScreen(screen)
	local allSpaces = hs.spaces.spacesForScreen(screen)

	for i, spaceId in ipairs(allSpaces) do
		if spaceId == activeSpace then
			return i
		end
	end
	return 1
end

-- Watcher, который отслеживает смену воркспейса
local spaceWatcher = hs.spaces.watcher.new(function()
	local newSpace = getCurrentSpaceNumber()
	if currentSpace and currentSpace ~= newSpace then
		previousSpace = currentSpace
		log.i(
			string.format(
				"Space changed: %s -> %s, previous is now: %s",
				tostring(currentSpace),
				tostring(newSpace),
				tostring(previousSpace)
			)
		)
	end
	currentSpace = newSpace
end)
spaceWatcher:start()

-- Инициализация при запуске
currentSpace = getCurrentSpaceNumber()
log.i("Initial space detected: " .. currentSpace)

function switchToSpace(spaceNumber)
	local current = getCurrentSpaceNumber()
	if current ~= spaceNumber then
		hs.eventtap.keyStroke({ "ctrl" }, tostring(spaceNumber))
		log.i(string.format("Switched to space %d from %d", spaceNumber, current))
	else
		log.i("Already on space " .. spaceNumber)
	end
end

function switchToPreviousSpace()
	if previousSpace then
		log.i("Switching back to previous space: " .. previousSpace)
		switchToSpace(previousSpace)
	else
		log.w("No previous space saved to switch back to.")
		hs.alert.show("No previous space")
	end
end

hs.hotkey.bind({ "alt" }, "tab", function()
	switchToPreviousSpace()
end)

-- Уведомление о том, что конфиг успешно загружен
hs.alert.show("Hammerspoon config fully loaded!")
