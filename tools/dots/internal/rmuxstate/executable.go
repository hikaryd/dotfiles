package rmuxstate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

func executableDirectorySafe(goos, path string, owner, group, mode, current uint32) bool {
	if owner != current && owner != 0 || mode&0002 != 0 {
		return false
	}
	if mode&0020 == 0 {
		return true
	}
	return goos == "darwin" && (path == "/opt/homebrew" || strings.HasPrefix(path, "/opt/homebrew/")) && owner == current && group == 80
}
func executableDir(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("unsafe executable directory")
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	current := ""
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		current += "/" + part
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
		var st unix.Stat_t
		if e = unix.Fstat(fd, &st); e != nil || !executableDirectorySafe(runtime.GOOS, current, st.Uid, st.Gid, uint32(st.Mode), uint32(os.Getuid())) {
			unix.Close(fd)
			return nil, errors.New("unsafe executable directory ownership or permissions")
		}
	}
	return os.NewFile(uintptr(fd), path), nil
}
