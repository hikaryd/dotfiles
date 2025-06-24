local log = hs.logger.new("WindowMover", "info")

function yabai(args)
	return hs.task.new("/opt/homebrew/bin/yabai", nil, args)
end

function runYabai(args)
	yabai(args):start()
end

-- Фокус окна (hjkl)
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

-- Перемещение окон
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

-- Баланс размеров
hs.hotkey.bind({ "alt", "shift" }, "0", function()
	runYabai({ "-m", "space", "--balance" })
end)

-- Перемещение между дисплеями
hs.hotkey.bind({ "ctrl", "alt" }, "h", function()
	runYabai({ "-m", "window", "--display", "west" })
	runYabai({ "-m", "display", "--focus", "west" })
end)

hs.hotkey.bind({ "ctrl", "alt" }, "l", function()
	runYabai({ "-m", "window", "--display", "east" })
	runYabai({ "-m", "display", "--focus", "east" })
end)

-- Фокус дисплеев
hs.hotkey.bind({ "cmd", "shift" }, "l", function()
	runYabai({ "-m", "display", "--focus", "next" })
end)

hs.hotkey.bind({ "cmd", "shift" }, "h", function()
	runYabai({ "-m", "display", "--focus", "prev" })
end)

-- Переключение на предыдущий space
hs.hotkey.bind({ "alt" }, "tab", function()
	switchToPreviousSpace()
end)

-- Перемещение окон на space
for i = 1, 8 do
	hs.hotkey.bind({ "alt", "shift" }, tostring(i), function()
		moveFocusedWindowToSpace(i)
	end)
end

-- Переключение между floating и tiling
hs.hotkey.bind({ "cmd", "shift" }, "f", function()
	runYabai({ "-m", "window", "--toggle", "float" })
end)

-- Закрытие окна
hs.hotkey.bind({ "alt" }, "q", function()
	runYabai({ "-m", "window", "--close" })
end)

-- ====================
-- ПЕРЕКЛЮЧЕНИЕ НА ПРЕДЫДУЩИЙ SPACE
-- ====================

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

-- Watcher для отслеживания изменений spaces
local spaceWatcher = hs.spaces.watcher.new(function()
	local newSpace = getCurrentSpaceNumber()
	if currentSpace and currentSpace ~= newSpace then
		previousSpace = currentSpace
		log.i(
			"Space changed: "
				.. (currentSpace or "nil")
				.. " -> "
				.. newSpace
				.. ", previous: "
				.. (previousSpace or "nil")
		)
	end
	currentSpace = newSpace
end)
spaceWatcher:start()

currentSpace = getCurrentSpaceNumber()
log.i("Initial space: " .. currentSpace)

function switchToSpace(spaceNumber)
	local current = getCurrentSpaceNumber()
	if current ~= spaceNumber then
		previousSpace = current
		log.i("Manual switch: " .. current .. " -> " .. spaceNumber .. ", saving previous: " .. previousSpace)
		hs.eventtap.keyStroke({ "alt" }, tostring(spaceNumber), 200000)
	end
end

function switchToPreviousSpace()
	if previousSpace then
		log.i("Switching to previous space: " .. previousSpace)
		switchToSpace(previousSpace)
	else
		log.w("No previous space saved")
		hs.alert.show("No previous space")
	end
end

-- ====================
-- ТВОЯ ФУНКЦИЯ ДЛЯ ПЕРЕМЕЩЕНИЯ ОКОН НА SPACES
-- ====================

-- Move window to space
function moveWindowToSpace(window, spaceNumber)
	log.i("Moving window " .. window:title() .. " to space " .. spaceNumber)
	local prevCursorPoint = hs.mouse.absolutePosition()
	local winFrame = window:frame()
	local point = hs.geometry(winFrame.x + 5, winFrame.y + 15)

	local module = hs.eventtap
	module.event.newMouseEvent(module.event.types["leftMouseDown"], point):post()
	hs.timer.doAfter(1, function()
		module.event.newMouseEvent(module.event.types["leftMouseUp"], point):post()
		hs.mouse.absolutePosition(prevCursorPoint)
	end)

	-- Switch to target space with Mission Control shortcuts
	if spaceNumber < 10 then
		hs.eventtap.keyStroke({ "option" }, tostring(spaceNumber), 200000)
	else
		hs.eventtap.keyStroke({ "ctrl", "alt" }, tostring(spaceNumber - 10), 200000)
	end
end

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

-- Reload конфига
hs.hotkey.bind({ "cmd", "alt", "ctrl" }, "r", function()
	hs.reload()
	hs.alert.show("Config reloaded")
end)

hs.alert.show("Hammerspoon config loaded!")
