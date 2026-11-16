package metrics

import (
	"errors"
	"strings"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// Snapshot is one sample of metric values.
type Snapshot struct {
	Time   time.Time
	Values map[string]float64
}

// Sample reads live counters under r. Stub.
func Sample(r facts.Root) (*Snapshot, error) {
	_ = r
	return nil, errors.New("not implemented")
}

// IsCounter reports whether key is a monotonically increasing counter.
// Gauges are conntrack.*, fs.file_nr.*, sockstat.*, psi.*.
func IsCounter(key string) bool {
	for _, p := range []string{"conntrack.", "fs.file_nr.", "sockstat.", "psi."} {
		if strings.HasPrefix(key, p) {
			return false
		}
	}
	return true
}

// Rates returns per-second rates of counters between prev and cur.
// Keys missing from either side, non-counters, and negative deltas (counter
// resets) are skipped. Returns an empty map for nil snapshots or dt <= 0.
func Rates(prev, cur *Snapshot) map[string]float64 {
	out := map[string]float64{}
	if prev == nil || cur == nil {
		return out
	}
	dt := cur.Time.Sub(prev.Time).Seconds()
	if dt <= 0 {
		return out
	}
	for k, c := range cur.Values {
		if !IsCounter(k) {
			continue
		}
		p, ok := prev.Values[k]
		if !ok || c < p {
			continue
		}
		out[k] = (c - p) / dt
	}
	return out
}
