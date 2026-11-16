package facts

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func collectCPU(r Root, f *Facts) {
	c := &f.CPU
	procs, physIDs, coresPerSock := parseCPUInfo(r, f)

	sysCPU := filepath.Join(r.Sys, "devices/system/cpu")
	ids := numberedDirs(sysCPU, "cpu")
	pkgs := map[string]bool{}
	cores := map[string]bool{}
	for _, id := range ids {
		top := filepath.Join(sysCPU, "cpu"+strconv.Itoa(id), "topology")
		pkg, err := readStr(filepath.Join(top, "physical_package_id"))
		if err != nil {
			continue
		}
		core := readOpt(filepath.Join(top, "core_id"))
		pkgs[pkg] = true
		cores[pkg+"/"+core] = true
		c.Threads++
	}
	if c.Threads > 0 {
		c.Sockets, c.Cores = len(pkgs), len(cores)
	} else {
		f.warnf("no cpu topology under %s; falling back to cpuinfo", sysCPU)
		c.Threads = procs
		c.Sockets = max(len(physIDs), 1)
		c.Cores = c.Sockets * coresPerSock
		if coresPerSock == 0 {
			c.Cores = procs
		}
	}

	c.SMTActive = readOpt(filepath.Join(sysCPU, "smt/active")) == "1"
	c.SMTControl = readOpt(filepath.Join(sysCPU, "smt/control"))
	cf := filepath.Join(sysCPU, "cpu0/cpufreq")
	c.ScalingDriver = readOpt(filepath.Join(cf, "scaling_driver"))
	c.Governor = readOpt(filepath.Join(cf, "scaling_governor"))
	c.EPP = readOpt(filepath.Join(cf, "energy_performance_preference"))
	c.IdleDriver = readOpt(filepath.Join(sysCPU, "cpuidle/current_driver"))
}

// parseCPUInfo fills Vendor, Model and Flags from the first processor block.
// It returns the processor count, the set of physical ids and "cpu cores".
func parseCPUInfo(r Root, f *Facts) (int, map[string]bool, int) {
	b, err := os.ReadFile(filepath.Join(r.Proc, "cpuinfo"))
	if err != nil {
		f.warnf("cannot read %s: %v", filepath.Join(r.Proc, "cpuinfo"), err)
		return 0, nil, 0
	}
	c := &f.CPU
	procs, coresPerSock := 0, 0
	phys := map[string]bool{}
	first := true
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			if strings.TrimSpace(line) == "" && procs > 0 {
				first = false
			}
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			procs++
		case "physical id":
			phys[v] = true
		case "cpu cores":
			coresPerSock = atoi(v)
		}
		if !first {
			continue
		}
		switch k {
		case "vendor_id":
			c.Vendor = v
		case "model name", "Model Name", "cpu model":
			if c.Model == "" {
				c.Model = v
			}
		case "flags", "Features":
			for _, fl := range strings.Fields(v) {
				c.Flags[fl] = true
			}
		}
	}
	if c.Vendor == "" && c.Flags["fp"] { // arm: no vendor_id
		for _, line := range strings.Split(string(b), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "CPU implementer" {
				c.Vendor = "ARM:" + strings.TrimSpace(v)
				break
			}
		}
	}
	if procs == 0 {
		f.warnf("no processors in cpuinfo")
	}
	return procs, phys, coresPerSock
}
