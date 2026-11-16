package facts

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func collectNUMA(r Root, f *Facts) {
	base := filepath.Join(r.Sys, "devices/system/node")
	ids := numberedDirs(base, "node")
	if len(ids) == 0 {
		f.warnf("no NUMA nodes under %s", base)
		return
	}
	for _, id := range ids {
		dir := filepath.Join(base, "node"+strconv.Itoa(id))
		n := NUMANode{ID: id, CPUs: parseCPUList(readOpt(filepath.Join(dir, "cpulist")))}
		if b, err := os.ReadFile(filepath.Join(dir, "meminfo")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				// "Node 0 MemTotal:  65793164 kB"
				fs := strings.Fields(line)
				if len(fs) >= 4 && fs[2] == "MemTotal:" {
					n.MemTotalBytes = atou(fs[3]) * 1024
				}
			}
		}
		f.NUMA = append(f.NUMA, n)
	}
}

func collectMemory(r Root, f *Facts) {
	m := &f.Memory
	b, err := os.ReadFile(filepath.Join(r.Proc, "meminfo"))
	if err != nil {
		f.warnf("cannot read %s: %v", filepath.Join(r.Proc, "meminfo"), err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fs := strings.Fields(v)
		if len(fs) == 0 {
			continue
		}
		switch k {
		case "MemTotal":
			m.TotalBytes = atou(fs[0]) * 1024
		case "SwapTotal":
			m.SwapTotalBytes = atou(fs[0]) * 1024
		}
	}
	thp := filepath.Join(r.Sys, "kernel/mm/transparent_hugepage")
	if s, err := readStr(filepath.Join(thp, "enabled")); err == nil {
		m.THPEnabled, _ = bracketed(s)
	} else {
		f.warnf("cannot read THP settings under %s", thp)
	}
	if s, err := readStr(filepath.Join(thp, "defrag")); err == nil {
		m.THPDefrag, _ = bracketed(s)
	}
	hp := filepath.Join(r.Sys, "kernel/mm/hugepages")
	if ents, err := os.ReadDir(hp); err == nil {
		for _, e := range ents {
			size, ok := strings.CutPrefix(e.Name(), "hugepages-")
			if !ok {
				continue
			}
			if s, err := readStr(filepath.Join(hp, e.Name(), "nr_hugepages")); err == nil {
				m.HugePages[size] = atou(s)
			}
		}
	}
}

// collectSwaps parses /proc/swaps. A missing file means no swap data and is
// not worth a warning.
func collectSwaps(r Root, f *Facts) {
	b, err := os.ReadFile(filepath.Join(r.Proc, "swaps"))
	if err != nil {
		return
	}
	f.Swaps = parseSwaps(string(b))
}

func parseSwaps(s string) []Swap {
	var out []Swap
	for i, line := range strings.Split(s, "\n") {
		fs := strings.Fields(line)
		if i == 0 && len(fs) > 0 && fs[0] == "Filename" {
			continue
		}
		if len(fs) < 5 {
			continue
		}
		path := strings.ReplaceAll(fs[0], `\040`, " ")
		out = append(out, Swap{
			Path: path, Type: fs[1], SizeKB: atou(fs[2]), Priority: atoi(fs[4]),
			Zram: strings.HasPrefix(filepath.Base(path), "zram"),
		})
	}
	return out
}
