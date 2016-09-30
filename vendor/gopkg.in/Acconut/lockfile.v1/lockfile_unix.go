// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package lockfile

import (
	"os"
	"syscall"
)

func isRunning(pid int) (bool, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	} else {
		err := proc.Signal(syscall.Signal(0))
		if err == nil {
			return true, nil
		} else {
			return false, nil
		}
	}
}
