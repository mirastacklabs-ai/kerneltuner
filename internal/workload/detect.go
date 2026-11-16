package workload

import "github.com/mirastacklabs-ai/kerneltuner/internal/facts"

// Detect finds applications and listeners. Stub: returns an empty Workload.
func Detect(r facts.Root, f *facts.Facts) (*Workload, error) {
	_, _ = r, f
	return &Workload{}, nil
}
