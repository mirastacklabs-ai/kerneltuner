package metrics

import "testing"

// R-Q12: NaN and Inf never become metric values.
func TestNumDropsNaNAndInf(t *testing.T) {
	for _, s := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "infinity", "1e999"} {
		if f, ok := num(s); ok {
			t.Errorf("num(%q) = %v, want rejection", s, f)
		}
	}
	for s, want := range map[string]float64{"0": 0, "-1": -1, "2.5": 2.5, "1e3": 1000} {
		if f, ok := num(s); !ok || f != want {
			t.Errorf("num(%q) = %v %v", s, f, ok)
		}
	}
}
