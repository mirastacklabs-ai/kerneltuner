package apply

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// euid is a variable so tests can simulate root or a foreign owner.
var euid = os.Geteuid

// errSymlink marks a refused symlink or non-directory path component.
var errSymlink = errors.New("refusing to follow a symlink (or non-directory) in path")

// fileMeta is what the safe readers report about a file or directory.
type fileMeta struct {
	Mode uint32 // permission bits
	UID  uint32
}

func openBase(base string, create bool) (int, error) {
	fd, err := unix.Open(base, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if merr := os.MkdirAll(base, 0o755); merr != nil {
			return -1, merr
		}
		fd, err = unix.Open(base, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		return -1, &fs.PathError{Op: "open", Path: base, Err: err}
	}
	return fd, nil
}

// split returns the components of p below base. p must lie strictly inside
// base lexically (dir true allows p == base).
func split(base, p string, dirOK bool) ([]string, error) {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(p))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path %q is outside %q", p, base)
	}
	if rel == "." {
		if dirOK {
			return nil, nil
		}
		return nil, fmt.Errorf("path %q is the base directory", p)
	}
	return strings.Split(rel, string(filepath.Separator)), nil
}

// openDirBeneath opens dir (inside base) without following any symlink below
// base. With mkdir, missing directories are created (0755) and appended to
// created. The caller closes the returned fd.
func openDirBeneath(base, dir string, mkdir bool, created *[]string) (int, error) {
	comps, err := split(base, dir, true)
	if err != nil {
		return -1, err
	}
	cur, err := openBase(base, mkdir)
	if err != nil {
		return -1, err
	}
	if len(comps) == 0 {
		return cur, nil
	}
	if !mkdir {
		if fd, err := openat2Beneath(cur, filepath.Join(comps...), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC); err == nil {
			_ = unix.Close(cur)
			return fd, nil
		} else if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTDIR) {
			_ = unix.Close(cur)
			return -1, fmt.Errorf("%w: %s", errSymlink, dir)
		}
		// ENOENT is reported by the walk below; ENOSYS and the like fall back to it.
	}
	walked := base
	for _, c := range comps {
		walked = filepath.Join(walked, c)
		next, err := unix.Openat(cur, c, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) && mkdir {
			if err = unix.Mkdirat(cur, c, 0o755); err == nil {
				if created != nil {
					*created = append(*created, walked)
				}
				next, err = unix.Openat(cur, c, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		_ = unix.Close(cur)
		if err != nil {
			if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
				return -1, fmt.Errorf("%w: %s", errSymlink, walked)
			}
			return -1, &fs.PathError{Op: "open", Path: walked, Err: err}
		}
		cur = next
	}
	return cur, nil
}

func isLink(dirfd int, name string) (bool, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, err
	}
	return st.Mode&unix.S_IFMT == unix.S_IFLNK, nil
}

// readFileBeneath reads path (inside base) refusing symlinks everywhere.
func readFileBeneath(base, path string) ([]byte, fileMeta, error) {
	dfd, err := openDirBeneath(base, filepath.Dir(path), false, nil)
	if err != nil {
		return nil, fileMeta{}, err
	}
	defer func() { _ = unix.Close(dfd) }()
	if _, err := split(base, path, false); err != nil {
		return nil, fileMeta{}, err
	}
	fd, err := unix.Openat(dfd, filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, fileMeta{}, fmt.Errorf("%w: %s", errSymlink, path)
		}
		return nil, fileMeta{}, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, fileMeta{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, fileMeta{}, fmt.Errorf("%s is not a regular file", path)
	}
	b, err := readAll(f, 8<<20)
	return b, fileMeta{Mode: st.Mode & 0o7777, UID: st.Uid}, err
}

func readAll(f *os.File, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", f.Name(), limit)
	}
	return b, err
}

// writeFileBeneath atomically replaces path (inside base) via a temp file in
// the same directory, creating missing parent directories. It refuses to
// follow or replace a symlink at any component.
func writeFileBeneath(base, path string, data []byte, mode os.FileMode, created *[]string) error {
	if _, err := split(base, path, false); err != nil {
		return err
	}
	dfd, err := openDirBeneath(base, filepath.Dir(path), true, created)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(dfd) }()
	name := filepath.Base(path)
	if link, err := isLink(dfd, name); err == nil && link {
		return fmt.Errorf("%w: %s", errSymlink, path)
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	tmp := ".kerneltuner-" + hex.EncodeToString(rnd[:])
	fd, err := unix.Openat(dfd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return &fs.PathError{Op: "create", Path: filepath.Join(filepath.Dir(path), tmp), Err: err}
	}
	f := os.NewFile(uintptr(fd), tmp)
	cleanup := func() { _ = unix.Unlinkat(dfd, tmp, 0) }
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := unix.Fchmod(fd, uint32(mode&0o644)); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := unix.Renameat(dfd, tmp, dfd, name); err != nil {
		cleanup()
		return &fs.PathError{Op: "rename", Path: path, Err: err}
	}
	return nil
}

// removeBeneath unlinks path (inside base), refusing symlinks. A missing file
// is not an error. With dir set it removes an empty directory instead.
func removeBeneath(base, path string, dir bool) error {
	if _, err := split(base, path, dir); err != nil {
		return err
	}
	dfd, err := openDirBeneath(base, filepath.Dir(path), false, nil)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = unix.Close(dfd) }()
	name := filepath.Base(path)
	link, err := isLink(dfd, name)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	if link {
		return fmt.Errorf("%w: %s", errSymlink, path)
	}
	flags := 0
	if dir {
		flags = unix.AT_REMOVEDIR
	}
	if err := unix.Unlinkat(dfd, name, flags); err != nil && !errors.Is(err, unix.ENOENT) {
		return &fs.PathError{Op: "remove", Path: path, Err: err}
	}
	return nil
}

// statDirBeneath reports owner and permissions of a directory inside base.
func statDirBeneath(base, dir string) (fileMeta, error) {
	fd, err := openDirBeneath(base, dir, false, nil)
	if err != nil {
		return fileMeta{}, err
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fileMeta{}, err
	}
	return fileMeta{Mode: st.Mode & 0o7777, UID: st.Uid}, nil
}

// ensurePrivateDir creates dir (inside base) and makes sure it is owned by
// the current user and mode 0700. A directory owned by someone else is
// refused; one with looser permissions is chmodded when we own it.
func ensurePrivateDir(base, dir string) error {
	fd, err := openDirBeneath(base, dir, true, nil)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if int(st.Uid) != euid() {
		return fmt.Errorf("%s is owned by uid %d, not %d: refusing to use it", dir, st.Uid, euid())
	}
	if st.Mode&0o7777 != 0o700 {
		if err := unix.Fchmod(fd, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// listDirBeneath returns the entries of dir (inside base).
func listDirBeneath(base, dir string) ([]os.DirEntry, error) {
	fd, err := openDirBeneath(base, dir, false, nil)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), dir)
	defer func() { _ = f.Close() }()
	return f.ReadDir(-1)
}

// CheckRoot verifies a --root tree before a privileged run acts on it: the
// root directory and each of its proc, sys, etc and var bases must be real
// directories (opened with O_NOFOLLOW, so never a symlink), owned by root and
// not group- or world-writable. A base that does not exist yet is accepted:
// root creates it, and its parent has been checked. dir is the --root value.
func CheckRoot(dir string, root facts.Root) error {
	for _, d := range []string{dir, root.Proc, root.Sys, root.Etc, root.Var} {
		fd, err := unix.Open(d, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			switch {
			case errors.Is(err, unix.ENOENT) && d != dir:
				continue
			case errors.Is(err, unix.ELOOP), errors.Is(err, unix.ENOTDIR):
				return fmt.Errorf("refusing --root: %s is a symlink or not a directory", facts.SafeText(d))
			}
			return fmt.Errorf("refusing --root: %s: %w", facts.SafeText(d), err)
		}
		var st unix.Stat_t
		err = unix.Fstat(fd, &st)
		_ = unix.Close(fd)
		if err != nil {
			return fmt.Errorf("refusing --root: %s: %w", facts.SafeText(d), err)
		}
		if err := trusted(d, fileMeta{Mode: st.Mode & 0o7777, UID: st.Uid}); err != nil {
			return fmt.Errorf("refusing --root: %w", err)
		}
	}
	return nil
}
