package desktop

import (
	"os"
	"path/filepath"
)

// Compare recorded link destinations even after the old checkout disappears.
// Never infer ownership merely from a filename or remove foreign cache entries.
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
