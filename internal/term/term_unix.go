//go:build darwin || linux

package term

import (
	"runtime"

	"golang.org/x/sys/unix"
)

func ioctlGet() uint {
	if runtime.GOOS == "darwin" {
		return unix.TIOCGETA
	}
	return 0x5401 // TCGETS
}

func ioctlSet() uint {
	if runtime.GOOS == "darwin" {
		return unix.TIOCSETA
	}
	return 0x5402 // TCSETS
}

// IsTerminal — дескриптор смотрит в терминал.
func IsTerminal(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), ioctlGet())
	return err == nil
}

func noEcho(fd uintptr) (func(), error) {
	old, err := unix.IoctlGetTermios(int(fd), ioctlGet())
	if err != nil {
		return nil, err
	}
	t := *old
	t.Lflag &^= unix.ECHO
	t.Lflag |= unix.ICANON | unix.ISIG
	if err := unix.IoctlSetTermios(int(fd), ioctlSet(), &t); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(int(fd), ioctlSet(), old) }, nil
}
