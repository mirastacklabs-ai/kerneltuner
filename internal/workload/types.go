package workload

// Workload describes detected applications and listeners.
type Workload struct {
	Apps        []App
	Listeners   int
	ListenPorts []int
}

// App is a detected application. Name is canonical: nginx haproxy envoy redis
// valkey postgres mysql kafka elasticsearch vllm sglang tgi triton llama.cpp
// ollama pytorch jvm.
type App struct {
	Name        string
	PIDs        []int
	Ports       []int
	Unit        string
	NoFileSoft  uint64
	NoFileHard  uint64
	MemlockSoft uint64
	MaxOpenFDs  int
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
