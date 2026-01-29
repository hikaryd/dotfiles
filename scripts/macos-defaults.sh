#!/usr/bin/env bash

set -e

echo "Requesting sudo to apply macOS system preferences..."
if ! sudo -v; then
	echo "Skipping macOS defaults (no sudo credentials)."
	exit 0
fi

echo "Configuring macOS system preferences..."

# Global preferences

# Темная тема
defaults write NSGlobalDomain AppleInterfaceStyle -string "Dark"

# Всегда показывать расширения файлов
defaults write NSGlobalDomain AppleShowAllExtensions -bool true

# Скрывать меню-бар
defaults write NSGlobalDomain _HIHideMenuBar -bool true

# Отключать анимации окон
defaults write NSGlobalDomain NSAutomaticWindowAnimationsEnabled -bool false

# Скорость трекпада
defaults write NSGlobalDomain com.apple.trackpad.scaling -float 0.8

# Метрическая система
defaults write NSGlobalDomain AppleMeasurementUnits -string "Centimeters"
defaults write NSGlobalDomain AppleMetricUnits -int 1
defaults write NSGlobalDomain AppleTemperatureUnit -string "Celsius"

# Поведение клавиш повтора
defaults write NSGlobalDomain InitialKeyRepeat -int 15
defaults write NSGlobalDomain KeyRepeat -int 2

# Полосы прокрутки: показывать при скролле
defaults write NSGlobalDomain AppleShowScrollBars -string "WhenScrolling"

# Расширенный диалог сохранения
defaults write NSGlobalDomain NSNavPanelExpandedStateForSaveMode -bool true
defaults write NSGlobalDomain NSNavPanelExpandedStateForSaveMode2 -bool true

# Полный доступ клавиатурой: Tab переключает все элементы
defaults write NSGlobalDomain AppleKeyboardUIMode -int 3

# Текстовые «умные» фичи (отключить)
defaults write NSGlobalDomain NSAutomaticQuoteSubstitutionEnabled -bool false
defaults write NSGlobalDomain NSAutomaticSpellingCorrectionEnabled -bool false
defaults write NSGlobalDomain NSAutomaticCapitalizationEnabled -bool false
defaults write NSGlobalDomain NSAutomaticDashSubstitutionEnabled -bool false
defaults write NSGlobalDomain NSAutomaticPeriodSubstitutionEnabled -bool false

# Finder

# Показывать все расширения
defaults write com.apple.finder AppleShowAllExtensions -bool true

# Показывать скрытые файлы
defaults write com.apple.finder AppleShowAllFiles -bool true

# Папки выше файлов
defaults write com.apple.finder _FXSortFoldersFirst -bool true

# Панель пути внизу
defaults write com.apple.finder ShowPathbar -bool true

# Статус-бар скрыт
defaults write com.apple.finder ShowStatusBar -bool false

# Можно выйти из Finder через ⌘Q
defaults write com.apple.finder QuitMenuItem -bool true

# Полный POSIX-путь в заголовке
defaults write com.apple.finder _FXShowPosixPathInTitle -bool true

# Не спрашивать при смене расширения
defaults write com.apple.finder FXEnableExtensionChangeWarning -bool false

# Поиск по умолчанию: текущая папка
defaults write com.apple.finder FXDefaultSearchScope -string "SCcf"

# Dock

# Автоскрытие Dock
defaults write com.apple.dock autohide -bool true

# Показывать мгновенно
defaults write com.apple.dock autohide-delay -float 0.0

# Скорость анимации
defaults write com.apple.dock autohide-time-modifier -float 0.45

# Справа
defaults write com.apple.dock orientation -string "right"

# Без «Недавних»
defaults write com.apple.dock show-recents -bool false

# Очистить persistent-apps (начисто)
defaults write com.apple.dock persistent-apps -array

# Не пересортировывать рабочие столы
defaults write com.apple.dock mru-spaces -bool false

# Размер иконок
defaults write com.apple.dock tilesize -int 30

# Точки у запущенных приложений
defaults write com.apple.dock show-process-indicators -bool true

# Spaces
defaults write com.apple.spaces spans-displays -bool true

# Screencapture
USER=$(whoami)
mkdir -p "/Users/$USER/Desktop/screenshots"
defaults write com.apple.screencapture location "/Users/$USER/Desktop/screenshots"
defaults write com.apple.screencapture type "png"

# Startup

# Отключить звук при включении
sudo nvram StartupMute=%01 || true

# User library

# Показать Library
chflags nohidden "/Users/$USER/Library" 2>/dev/null || true

# Restart affected apps

echo "Restarting affected applications..."
killall Finder 2>/dev/null || true
killall Dock 2>/dev/null || true
killall SystemUIServer 2>/dev/null || true

echo "✅ macOS preferences configured successfully!"
echo "Note: Some changes may require a logout/restart to take full effect."
