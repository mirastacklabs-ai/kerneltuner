package facts

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func collectBlock(r Root, f *Facts) {
	base := filepath.Join(r.Sys, "block")
	ents, err := os.ReadDir(base)
	if err != nil {
		f.warnf("cannot read %s: %v", base, err)
		return
	}
	for _, e := range ents {
		name := e.Name()
		if SkipBlock(name) {
			continue
		}
		q := filepath.Join(base, name, "queue")
		b := BlockDev{
			Name:        name,
			Rotational:  readOpt(filepath.Join(q, "rotational")) == "1",
			ReadAheadKB: atoi(readOpt(filepath.Join(q, "read_ahead_kb"))),
			NrRequests:  atoi(readOpt(filepath.Join(q, "nr_requests"))),
		}
		b.Scheduler, b.Schedulers = bracketed(readOpt(filepath.Join(q, "scheduler")))
		b.Transport = blockTransport(name, readOpt(filepath.Join(base, name, "device/transport")))
		f.Block = append(f.Block, b)
	}
	sort.Slice(f.Block, func(i, j int) bool { return f.Block[i].Name < f.Block[j].Name })
}

// SkipBlock reports whether a block device is virtual or uninteresting
// (device-mapper is skipped too: its tuning belongs to the underlying disks).
func SkipBlock(name string) bool {
	for _, p := range []string{"loop", "ram", "zram", "dm-", "sr", "fd", "nbd"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func blockTransport(name, devTransport string) string {
	switch {
	case strings.HasPrefix(name, "nvme"):
		return "nvme"
	case strings.HasPrefix(name, "vd"):
		return "virtio"
	case strings.HasPrefix(name, "sd"), strings.HasPrefix(name, "hd"):
		return "sata"
	}
	if devTransport != "" {
		return devTransport
	}
	return ""
}
