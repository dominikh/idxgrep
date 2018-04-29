// +build !linux,!openbsd,!freebsd,!netbsd,!dragonflybsd,!darwin,!solaris,!windows

package exit

func isIOError(err error) bool { return false }
