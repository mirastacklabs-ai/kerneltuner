package workload

// Workload describes detected applications and listeners.
type Workload struct {
	Apps        []App
	Listeners   int
	ListenPorts []int
}

// App is a detected or declared application. Name is canonical; see KnownApps.
// Declared apps come from --workload and have no PIDs or ports.
type App struct {
	Name        string
	PIDs        []int
	Ports       []int
	Unit        string
	NoFileSoft  uint64 // minimum across the app's PIDs
	NoFileHard  uint64
	MemlockSoft uint64
	// NProcSoft is the minimum non-zero soft "Max processes" across the app's PIDs.
	NProcSoft uint64
	// OOMScoreAdj is the maximum /proc/<pid>/oom_score_adj across the app's PIDs
	// (the least protected process decides whether the app is OOM-protected).
	OOMScoreAdj int
	MaxOpenFDs  int
	// NoFileSoftMax is the maximum soft nofile across the app's PIDs: workers
	// raised above a low-limit master show here.
	NoFileSoftMax uint64
	// Container is the container runtime the app runs in (docker, containerd,
	// crio, podman), derived from its cgroup path; "" when not containerised.
	Container string
	Declared  bool // set via --workload, not detected
}

// Has reports whether an app with the given canonical name was detected.
func (w *Workload) Has(name string) bool {
	if w == nil {
		return false
	}
	for _, a := range w.Apps {
		if a.Name == name {
			return true
		}
	}
	return false
}
