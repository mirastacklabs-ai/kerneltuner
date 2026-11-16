package facts

// Collect gathers host facts under r. Stub: returns empty Facts.
func Collect(r Root) (*Facts, error) {
	_ = r
	return &Facts{
		Sysctl:  map[string]string{},
		Modules: map[string]bool{},
		Kernel:  Kernel{CmdlineArgs: map[string]string{}},
		CPU:     CPU{Flags: map[string]bool{}},
		Memory:  Memory{HugePages: map[string]uint64{}},
	}, nil
}
