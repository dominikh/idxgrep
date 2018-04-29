// +build linux openbsd freebsd netbsd dragonflybsd darwin solaris windows

package exit

import (
	"os"
	"syscall"
)

func isIOError(err error) bool {
	switch err := err.(type) {
	case *os.PathError:
		return isIOError(err.Err)
	case syscall.Errno:
		switch err {
		case syscall.EIO, syscall.EBUSY, syscall.ENOSPC, syscall.EPIPE:
			return true
		default:
			return false
		}
	default:
		return false
	}
}
