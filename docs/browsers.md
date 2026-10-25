# Браузеры

Сохранённый набор: **Brave, Safari, Google Chrome и Zephium**.
Brave остаётся браузером по умолчанию, пока это не изменено отдельно.

## Zephium

- Репозиторий: <https://github.com/zephium-browser/Zephium>.
- Релизы: <https://github.com/zephium-browser/Zephium/releases/latest>.
- macOS Apple Silicon: `Zephium-macOS-arm64.dmg`, macOS 14 или новее.
- Установка: открыть официальный DMG и скопировать `Zephium.app` в `/Applications`.
- Проверка загрузки: сверить SHA-256 с `SHA256SUMS` того же релиза.
- Проверка приложения: `codesign --verify --deep --strict /Applications/Zephium.app`.
- Обновления загружаются самим Zephium и применяются через **Relaunch to update**.

На 25 октября 2026 года установлен и запущен **Zephium 1.0.3**.
Официальный Homebrew cask `zephium` на эту дату отсутствует, поэтому установка
пока ручная: `./install` не скачивает Zephium автоматически. Эта заметка и ссылка
в `Brewfile` сохраняют его в списке используемых приложений.

## Убраны из активного набора

Firefox, Helium, Orion, Waterfox и Crest. Waterfox исключён из `Brewfile`
и автоматического применения настроек, чтобы `./install` не возвращал его
и не менял оставленные профили. Старые конфиги остаются в истории/репозитории.

При очистке 25 октября 2026 года приложения перенесены в Корзину без её очистки.
Пользовательские профили, закладки и пароли не удалялись и не копируются в Git.
