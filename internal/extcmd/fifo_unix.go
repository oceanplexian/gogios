//go:build !windows

package extcmd

import "syscall"

func mkfifoImpl(path string) error {
	return syscall.Mkfifo(path, 0660)
}
