package facts

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SysctlDirs are the /proc/sys subtrees read by Collect, non-recursively.
// Capture uses the same list.
var SysctlDirs = []string{
	"net/core", "net/ipv4", "net/netfilter", "vm", "kernel", "fs",
	"net/ipv4/conf/all", "net/ipv4/conf/default",
	"fs/inotify", "net/bridge",
}

// SysctlSkip names /proc/sys entries that are never read: they hold secrets
// or trigger work when read.
var SysctlSkip = map[string]bool{
	"hostname": true, "domainname": true, "tcp_fastopen_key": true, "stat_refresh": true,
	"boot_id": true, "uuid": true,
}

func collectSysctl(r Root, f *Facts) {
	base := filepath.Join(r.Proc, "sys")
	if _, err := os.Stat(base); err != nil {
		f.warnf("cannot read %s: %v", base, err)
		return
	}
	for _, d := range SysctlDirs {
		ents, err := os.ReadDir(filepath.Join(base, d))
		if err != nil {
			continue // e.g. net/netfilter without conntrack
		}
		prefix := strings.ReplaceAll(d, "/", ".") + "."
		for _, e := range ents {
			if !e.Type().IsRegular() || SysctlSkip[e.Name()] {
				continue
			}
			// Write-only entries (mode 0200) are skipped: root could open
			// them and trigger side effects.
			if info, err := e.Info(); err != nil || info.Mode().Perm()&0o444 == 0 {
				continue
			}
			b, err := ReadBounded(filepath.Join(base, d, e.Name()), procFileLimit)
			if err != nil {
				continue
			}
			f.Sysctl[prefix+e.Name()] = strings.Join(strings.Fields(string(b)), " ")
		}
	}
}

func collectLimits(r Root, f *Facts) {
	f.Limits.FileMax = atou(f.Sysctl["fs.file-max"])
	f.Limits.NrOpen = atou(f.Sysctl["fs.nr_open"])
	if s, ok := f.readReq(filepath.Join(r.Proc, "sys/fs/file-nr")); ok {
		if fs := strings.Fields(s); len(fs) > 0 {
			f.Limits.FileNrAllocated = atou(fs[0])
		}
	}
}

func collectModules(r Root, f *Facts) {
	b, err := os.ReadFile(filepath.Join(r.Proc, "modules"))
	if err != nil {
		f.warnf("cannot read %s: %v", filepath.Join(r.Proc, "modules"), err)
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		if fs := strings.Fields(line); len(fs) > 0 {
			f.Modules[fs[0]] = true
		}
	}
}

func collectProcs(r Root, f *Facts) {
	ents, err := os.ReadDir(r.Proc)
	if err != nil {
		f.warnf("cannot read %s: %v", r.Proc, err)
		return
	}
	for _, e := range ents {
		pid := atoiDef(e.Name(), -1)
		if pid < 0 || !e.IsDir() {
			continue
		}
		if len(f.Procs) >= maxProcs {
			f.warnf("more than %d processes: ignoring the rest", maxProcs)
			break
		}
		dir := filepath.Join(r.Proc, e.Name())
		comm, err := readStr(filepath.Join(dir, "comm"))
		if err != nil {
			continue // process exited while scanning
		}
		p := Proc{PID: pid, Comm: SafeText(comm)}
		if b, err := ReadBounded(filepath.Join(dir, "cmdline"), procFileLimit); err == nil {
			p.Cmdline = splitNUL(b)
			for i := range p.Cmdline {
				p.Cmdline[i] = SafeText(p.Cmdline[i])
			}
		}
		if b, err := ReadBounded(filepath.Join(dir, "limits"), procFileLimit); err == nil {
			p.NoFileSoft, p.NoFileHard = parseLimit(string(b), "Max open files")
			p.MemlockSoft, p.MemlockHard = parseLimit(string(b), "Max locked memory")
			p.NProcSoft, p.NProcHard = parseLimit(string(b), "Max processes")
			p.CoreSoft, _ = parseLimit(string(b), "Max core file size")
		}
		p.OOMScoreAdj = atoi(readOpt(filepath.Join(dir, "oom_score_adj")))
		p.OpenFDs = countFDs(filepath.Join(dir, "fd"))
		if b, err := ReadBounded(filepath.Join(dir, "cgroup"), procFileLimit); err == nil {
			p.Unit = unitFromCgroup(string(b))
			p.Container = containerFromCgroup(string(b))
		}
		f.Procs = append(f.Procs, p)
	}
	sort.Slice(f.Procs, func(i, j int) bool { return f.Procs[i].PID < f.Procs[j].PID })
}

func splitNUL(b []byte) []string {
	s := strings.TrimRight(string(b), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// parseLimit finds the row starting with name in /proc/<pid>/limits and
// returns soft and hard values; "unlimited" is math.MaxUint64.
func parseLimit(limits, name string) (soft, hard uint64) {
	for _, line := range strings.Split(limits, "\n") {
		rest, ok := strings.CutPrefix(line, name)
		if !ok {
			continue
		}
		fs := strings.Fields(rest)
		if len(fs) < 2 {
			return 0, 0
		}
		return limitVal(fs[0]), limitVal(fs[1])
	}
	return 0, 0
}

func limitVal(s string) uint64 {
	if s == "unlimited" {
		return math.MaxUint64
	}
	return atou(s)
}

// countFDs counts entries in fd/. Permission denied yields -1; a missing
// directory yields 0.
func countFDs(dir string) int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsPermission(err) {
			return -1
		}
		return 0
	}
	return len(ents)
}

// unitFromCgroup returns the systemd service unit a process belongs to.
// The cgroup path is attacker-influenced (user@*.service subtrees are
// delegated to users), so a unit is only accepted when the path runs
// system.slice[/<name>.slice]*/<name>.service, the name passes
// ValidUnitName, and nothing under user.slice or user@*.service counts.
func unitFromCgroup(cg string) string {
	for _, line := range strings.Split(cg, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		var els []string
		for _, e := range strings.Split(parts[2], "/") {
			if e != "" {
				els = append(els, e)
			}
		}
		if len(els) < 2 || els[0] != "system.slice" {
			continue
		}
		for _, el := range els[1:] {
			if strings.HasSuffix(el, ".slice") && !strings.HasPrefix(el, "user") {
				continue
			}
			if strings.HasSuffix(el, ".service") && !strings.HasPrefix(el, "user@") && ValidUnitName(el) {
				return el
			}
			break // a scope, a plain subgroup, or a bad name: no unit
		}
	}
	return ""
}

// containerFromCgroup derives the container runtime from cgroup paths such as
// .../docker-<id>.scope, .../cri-containerd-<id>.scope, .../crio-<id>.scope,
// .../libpod-<id>.scope or cgroup v1 /docker/<id>. "" when none matches.
func containerFromCgroup(cg string) string {
	for _, line := range strings.Split(cg, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		els := strings.Split(parts[2], "/")
		for i, el := range els {
			switch {
			case strings.HasPrefix(el, "docker-") && strings.HasSuffix(el, ".scope"):
				return "docker"
			case strings.HasPrefix(el, "cri-containerd-"):
				return "containerd"
			case strings.HasPrefix(el, "crio-"):
				return "crio"
			case strings.HasPrefix(el, "libpod-"):
				return "podman"
			case el == "docker" && i+1 < len(els) && els[i+1] != "":
				return "docker"
			}
		}
	}
	return ""
}

func collectEcosystem(r Root, f *Facts) {
	tunedRunning := false
	for _, p := range f.Procs {
		switch p.Comm {
		case "tuned":
			tunedRunning = true
		case "bpftune":
			f.Bpftune = true
		case "irqbalance":
			f.Irqbalance = true
		}
	}
	f.Tuned.Active = tunedRunning
	if s := readOpt(filepath.Join(r.Etc, "tuned/active_profile")); s != "" {
		f.Tuned.Profile = strings.Fields(s)[0]
	}
}
