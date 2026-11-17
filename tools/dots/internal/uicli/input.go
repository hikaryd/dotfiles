package uicli

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// stdin может быть блокирующим, и os.File.Close не прервёт Read. Поэтому ждём
// готовности короткими интервалами, чтобы после отмены не оставить фоновое
// чтение. Других читателей stdin в этом CLI нет.
func readInput(ctx context.Context) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	data := make([]byte, 0, 4096)
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var ready unix.FdSet
		ready.Set(fd)
		timeout := unix.NsecToTimeval(100_000_000)
		n, err := unix.Select(fd+1, &ready, nil, nil, &timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}
		n, err = unix.Read(fd, buffer[:min(len(buffer), 1024*1024+1-len(data))])
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return data, nil
		}
		data = append(data, buffer[:n]...)
		if len(data) > 1024*1024 {
			return data, nil
		}
	}
}
