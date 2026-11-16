package facts

import "fmt"

// Collect gathers host facts under r. Every path goes through r. A file that
// cannot be read leaves zero values and adds an entry to Facts.Warnings; Collect
// itself only fails on programming errors (currently never).
func Collect(r Root) (*Facts, error) {
	f := &Facts{
		Sysctl:  map[string]string{},
		Modules: map[string]bool{},
		Kernel:  Kernel{CmdlineArgs: map[string]string{}},
		CPU:     CPU{Flags: map[string]bool{}, Vulnerabilities: map[string]string{}},
		Memory:  Memory{HugePages: map[string]uint64{}},
	}
	// Order matters: host and accel look at cpuinfo/cmdline results.
	collectKernel(r, f)
	collectKconfig(r, f)
	collectCPU(r, f)
	collectHost(r, f)
	collectNUMA(r, f)
	collectMemory(r, f)
	collectNICs(r, f)
	collectBlock(r, f)
	collectAccel(r, f)
	collectRDMA(r, f)
	collectIOMMU(r, f)
	collectSysctl(r, f)
	collectLimits(r, f)
	collectModules(r, f)
	collectSwaps(r, f)
	collectMounts(r, f)
	collectProcs(r, f)
	collectEcosystem(r, f)
	return f, nil
}

func (f *Facts) warnf(format string, a ...any) {
	f.Warnings = append(f.Warnings, fmt.Sprintf(format, a...))
}
