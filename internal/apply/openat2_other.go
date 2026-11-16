//go:build !linux

package apply

import "golang.org/x/sys/unix"

func openat2Beneath(int, string, int) (int, error) { return -1, unix.ENOSYS }
