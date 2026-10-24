package podanalyze

import (
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Retention is an age ceiling, with a sample ceiling to keep memory bounded.
// The on-disk log is compacted at startup and every five minutes.
const maxHistorySamples = 100000

func updateHistory(path string, cutoff time.Time, item *sample, prune bool) ([]sample, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	// Never hang a dashboard behind another history writer.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if !prune && item != nil {
		return nil, appendHistory(path, *item)
	}
	history, err := loadHistory(path, cutoff)
	if err != nil {
		return nil, err
	}
	if item != nil {
		history = append(history, *item)
	}
	if len(history) > maxHistorySamples {
		history = history[len(history)-maxHistorySamples:]
	}
	return history, rewriteHistory(path, history)
}
