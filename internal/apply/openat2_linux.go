package apply

import "golang.org/x/sys/unix"

// openat2Beneath opens rel below dirfd with RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS
// (Linux 5.6+). Callers fall back to a component walk on ENOSYS/EPERM.
func openat2Beneath(dirfd int, rel string, flags int) (int, error) {
	return unix.Openat2(dirfd, rel, &unix.OpenHow{
		Flags:   uint64(flags),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
}
