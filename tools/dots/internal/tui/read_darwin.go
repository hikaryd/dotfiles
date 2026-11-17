package tui

import (
	"os"

	"golang.org/x/sys/unix"
)

// Чтение tty с таймаутом работает и на macOS /dev/tty, где нет poll(2).
// После отмены не остаётся читателя, конкурирующего со следующим экраном.
// Исходный termios восстанавливает cleanup из Raw.
func setReadTimeout(file *os.File) error {
	state, err := unix.IoctlGetTermios(int(file.Fd()), unix.TIOCGETA)
	if err != nil {
		return err
	}
	state.Cc[unix.VMIN] = 0
	state.Cc[unix.VTIME] = 1
	return unix.IoctlSetTermios(int(file.Fd()), unix.TIOCSETA, state)
}
