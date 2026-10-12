package tui

import (
	"os"

	"golang.org/x/sys/unix"
)

// A bounded tty read is portable even for macOS /dev/tty, which does not
// support poll(2). Cancellation cannot leave a reader competing with the next
// wizard screen. The original termios is restored by Raw's cleanup function.
func setReadTimeout(file *os.File) error {
	state, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}
	state.Cc[unix.VMIN] = 0
	state.Cc[unix.VTIME] = 1
	return unix.IoctlSetTermios(int(file.Fd()), unix.TCSETS, state)
}
