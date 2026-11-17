package desktop

import (
	"os"
	"path/filepath"
)

// Сравниваем записанные цели ссылок, даже если старого checkout уже нет.
// Владение по имени файла не определяем, чужие записи кэша не удаляем.
func ownedBridgeLink(path, current, previous string) bool {
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target) == filepath.Clean(current) ||
		previous != "" && filepath.Clean(target) == filepath.Clean(previous)
}
