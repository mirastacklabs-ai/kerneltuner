package facts

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Mount is one row of /proc/self/mounts.
type Mount struct {
	Device     string
	MountPoint string
	FSType     string
	Options    []string
	// Disk is the parent whole-disk name (nvme0n1, sda) of a /dev device,
	// resolved through <Sys>; "" for pseudo and network filesystems or when it
	// cannot be resolved. Device-mapper and md devices resolve to their first
	// underlying disk.
	Disk string
}

// HasOpt reports whether the mount has option opt. An opt without "=" also
// matches the key of a key=value option ("size" matches "size=1G").
func (m Mount) HasOpt(opt string) bool {
	for _, o := range m.Options {
		if o == opt {
			return true
		}
		if !strings.Contains(opt, "=") {
			if k, _, ok := strings.Cut(o, "="); ok && k == opt {
				return true
			}
		}
	}
	return false
}

// MountOf returns the mount that holds path: the longest mount point that is
// a path-component prefix of path (the last one listed wins for stacked
// mounts). The zero Mount (empty MountPoint) means there is no mount table.
func (f *Facts) MountOf(path string) Mount {
	p := filepath.Clean("/" + strings.TrimPrefix(path, "/"))
	best, bestLen := -1, -1
	for i, m := range f.Mounts {
		mp := filepath.Clean("/" + strings.TrimPrefix(m.MountPoint, "/"))
		if mp != "/" && p != mp && !strings.HasPrefix(p, mp+"/") {
			continue
		}
		if len(mp) >= bestLen {
			best, bestLen = i, len(mp)
		}
	}
	if best < 0 {
		return Mount{}
	}
	return f.Mounts[best]
}

// BlockDeviceOf maps a path (usually a mount point) to the name of the whole
// disk holding it, as listed in Facts.Block ("" when unknown).
func (f *Facts) BlockDeviceOf(path string) string { return f.MountOf(path).Disk }

func collectMounts(r Root, f *Facts) {
	var b []byte
	var err error
	for _, rel := range []string{"self/mounts", "mounts"} {
		b, err = os.ReadFile(filepath.Join(r.Proc, rel))
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			f.warnf("cannot read %s: %v", filepath.Join(r.Proc, rel), err)
			return
		}
	}
	if err != nil {
		return // optional: absent in fixtures and minimal containers
	}
	f.Mounts = parseMounts(string(b))
	for i := range f.Mounts {
		if strings.HasPrefix(f.Mounts[i].Device, "/dev/") {
			f.Mounts[i].Disk = DiskOf(r, f.Mounts[i].Device)
		}
	}
}

// parseMounts parses /proc/self/mounts lines "dev mp fstype opts dump pass".
func parseMounts(s string) []Mount {
	var out []Mount
	for _, line := range strings.Split(s, "\n") {
		fs := strings.Fields(line)
		if len(fs) < 4 {
			continue
		}
		m := Mount{Device: unescapeMount(fs[0]), MountPoint: unescapeMount(fs[1]), FSType: fs[2]}
		for _, o := range strings.Split(fs[3], ",") {
			if o != "" {
				m.Options = append(m.Options, o)
			}
		}
		out = append(out, m)
	}
	return out
}

// unescapeMount decodes the \040 style octal escapes of the mounts file.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// DiskOf resolves a /dev path to its parent whole-disk name using sysfs.
// A partition resolves to the disk above it (the parent of
// <Sys>/class/block/<part>); /dev/mapper/<name> and /dev/dm-N resolve to the
// first slave under <Sys>/block/dm-N/slaves, and md devices likewise. It
// returns "" when the device cannot be resolved.
func DiskOf(r Root, dev string) string {
	return diskOf(r, strings.TrimPrefix(dev, "/dev/"), 0)
}

func diskOf(r Root, name string, depth int) string {
	if name == "" || depth > 8 {
		return ""
	}
	if rest, ok := strings.CutPrefix(name, "mapper/"); ok {
		if name = dmByName(r, rest); name == "" {
			return ""
		}
	}
	name = filepath.Base(name)
	if slaves, err := os.ReadDir(filepath.Join(r.Sys, "block", name, "slaves")); err == nil && len(slaves) > 0 {
		return diskOf(r, slaves[0].Name(), depth+1)
	}
	// /sys/class/block/<part> links to .../<disk>/<part>; a whole disk links
	// to .../<disk>, whose parent is not itself a block device.
	if t, err := os.Readlink(filepath.Join(r.Sys, "class/block", name)); err == nil {
		parts := strings.Split(filepath.ToSlash(filepath.Clean(t)), "/")
		if n := len(parts); n >= 2 && exists(filepath.Join(r.Sys, "block", parts[n-2])) {
			return parts[n-2]
		}
		return name
	}
	if exists(filepath.Join(r.Sys, "block", name)) {
		return name
	}
	return ""
}

func dmByName(r Root, name string) string {
	ents, err := os.ReadDir(filepath.Join(r.Sys, "block"))
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "dm-") && readOpt(filepath.Join(r.Sys, "block", e.Name(), "dm/name")) == name {
			return e.Name()
		}
	}
	return ""
}
