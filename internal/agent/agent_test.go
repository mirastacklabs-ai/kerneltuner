package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/metrics"
)

type fakeClock struct {
	now time.Time
	ch  chan time.Time
}

func (c *fakeClock) Now() time.Time                       { return c.now }
func (c *fakeClock) After(time.Duration) <-chan time.Time { return c.ch }

type memSink struct{ events []Event }

func (m *memSink) Emit(e Event) error { m.events = append(m.events, e); return nil }

type harness struct {
	a    *Agent
	clk  *fakeClock
	sink *memSink
	hit  bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{clk: &fakeClock{now: time.Unix(1_000_000, 0), ch: make(chan time.Time)}, sink: &memSink{}}
	h.a = New(Config{
		Interval:  15 * time.Second,
		StatePath: filepath.Join(t.TempDir(), "run", "kt", "state.json"),
		Host:      "h1",
		Clock:     h.clk,
		Sink:      h.sink,
		Sample: func() (*metrics.Snapshot, error) {
			return &metrics.Snapshot{Time: h.clk.now, Values: map[string]float64{"x": 1}}, nil
		},
		Evaluate: func(_, _ map[string]float64) []advisor.Recommendation {
			r := advisor.Recommendation{RuleID: "net.core.somaxconn", Param: "net.core.somaxconn", Target: "", Title: "Listen backlog",
				Current: "128", Proposed: "4096", Status: advisor.StatusRecommend}
			if h.hit {
				r.Evidence = []advisor.EvidenceHit{{Metric: "TcpExt.ListenOverflows", Value: 3, Threshold: 0}}
			}
			return []advisor.Recommendation{r}
		},
	})
	return h
}

func (h *harness) tick(hit bool, n int) {
	h.hit = hit
	for i := 0; i < n; i++ {
		h.clk.now = h.clk.now.Add(15 * time.Second)
		h.a.Tick()
	}
}

func states(m *memSink) string {
	var s []string
	for _, e := range m.events {
		s = append(s, e.State)
	}
	return strings.Join(s, ",")
}

func TestFiresAfterTwoTicksResolvesAfterFour(t *testing.T) {
	h := newHarness(t)
	h.tick(true, 1)
	if len(h.sink.events) != 0 {
		t.Fatal("must not fire after 1 hit")
	}
	h.tick(true, 1)
	if states(h.sink) != "firing" {
		t.Fatalf("got %q", states(h.sink))
	}
	h.tick(true, 5) // stays firing, no duplicates
	if states(h.sink) != "firing" {
		t.Fatalf("got %q", states(h.sink))
	}
	h.tick(false, 3)
	if states(h.sink) != "firing" {
		t.Fatalf("3 clean ticks must not resolve: %q", states(h.sink))
	}
	h.tick(false, 1)
	if states(h.sink) != "firing,resolved" {
		t.Fatalf("got %q", states(h.sink))
	}
	e := h.sink.events[0]
	if e.RuleID != "net.core.somaxconn" || e.Current != "128" || e.Proposed != "4096" || !strings.Contains(e.Evidence, "TcpExt.ListenOverflows=3") {
		t.Fatalf("%+v", e)
	}
}

func TestInterruptedStreakResets(t *testing.T) {
	h := newHarness(t)
	h.tick(true, 1)
	h.tick(false, 1)
	h.tick(true, 1)
	if len(h.sink.events) != 0 {
		t.Fatal("non-consecutive hits must not fire")
	}
	h.tick(true, 1)
	h.tick(false, 3)
	h.tick(true, 1) // clean streak broken
	h.tick(false, 3)
	if states(h.sink) != "firing" {
		t.Fatalf("got %q", states(h.sink))
	}
}

func TestReAlertLimitedToOncePerHour(t *testing.T) {
	h := newHarness(t)
	h.tick(true, 2)  // firing
	h.tick(false, 4) // resolved
	h.tick(true, 2)  // fires again inside the hour: suppressed
	if states(h.sink) != "firing,resolved" {
		t.Fatalf("got %q", states(h.sink))
	}
	h.tick(false, 4) // resolve of a suppressed episode is silent too
	if states(h.sink) != "firing,resolved" {
		t.Fatalf("got %q", states(h.sink))
	}
	h.clk.now = h.clk.now.Add(time.Hour)
	h.tick(true, 2)
	if states(h.sink) != "firing,resolved,firing" {
		t.Fatalf("after an hour: %q", states(h.sink))
	}
}

func TestStatusFiringWithoutEvidence(t *testing.T) {
	h := newHarness(t)
	h.a.cfg.Evaluate = func(_, _ map[string]float64) []advisor.Recommendation {
		return []advisor.Recommendation{{RuleID: "anti.x", Target: "eth0", Status: advisor.StatusFiring}}
	}
	h.tick(true, 2)
	if states(h.sink) != "firing" || h.sink.events[0].Target != "eth0" {
		t.Fatalf("%+v", h.sink.events)
	}
}

func TestTargetsTrackedSeparately(t *testing.T) {
	h := newHarness(t)
	n := 0
	h.a.cfg.Evaluate = func(_, _ map[string]float64) []advisor.Recommendation {
		n++
		out := []advisor.Recommendation{{RuleID: "r", Target: "eth1", Evidence: []advisor.EvidenceHit{{Metric: "m"}}}}
		if n <= 2 {
			out = append(out, advisor.Recommendation{RuleID: "r", Target: "eth0", Evidence: []advisor.EvidenceHit{{Metric: "m"}}})
		}
		return out
	}
	h.tick(true, 2)
	if len(h.sink.events) != 2 {
		t.Fatalf("%+v", h.sink.events)
	}
	h.tick(true, 4)
	if states(h.sink) != "firing,firing,resolved" || h.sink.events[2].Target != "eth0" {
		t.Fatalf("%+v", h.sink.events)
	}
}

func TestStateFile(t *testing.T) {
	h := newHarness(t)
	h.tick(true, 2)
	sf, err := ReadState(h.a.cfg.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if sf.Host != "h1" || sf.Interval != "15s" || len(sf.Rules) != 1 || sf.Rules[0].State != StateFiring ||
		sf.Rules[0].Proposed != "4096" || sf.Rules[0].Since.IsZero() || len(sf.RecentEvents) != 1 {
		t.Fatalf("%+v", sf)
	}
	ents, _ := os.ReadDir(filepath.Dir(h.a.cfg.StatePath))
	if len(ents) != 1 {
		t.Fatalf("leftover temp files: %v", ents)
	}
	info, _ := os.Stat(filepath.Dir(h.a.cfg.StatePath))
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("dir mode %v", info.Mode().Perm())
	}
}

func TestRecentEventsCapped(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 40; i++ {
		h.clk.now = h.clk.now.Add(2 * time.Hour) // defeat the re-alert limit
		h.tick(true, 2)
		h.tick(false, 4)
	}
	sf, _ := ReadState(h.a.cfg.StatePath)
	if len(sf.RecentEvents) != MaxEvents {
		t.Fatalf("got %d", len(sf.RecentEvents))
	}
}

func TestSampleErrorSkipsTick(t *testing.T) {
	h := newHarness(t)
	var errs []error
	h.a.cfg.OnError = func(e error) { errs = append(errs, e) }
	h.a.cfg.Sample = func() (*metrics.Snapshot, error) { return nil, errors.New("boom") }
	h.tick(true, 3)
	if len(h.sink.events) != 0 || len(errs) != 3 {
		t.Fatalf("%v %v", h.sink.events, errs)
	}
}

func TestRatesPassedToEvaluate(t *testing.T) {
	h := newHarness(t)
	v := 0.0
	h.a.cfg.Sample = func() (*metrics.Snapshot, error) {
		v += 30
		return &metrics.Snapshot{Time: h.clk.now, Values: map[string]float64{"TcpExt.ListenOverflows": v}}, nil
	}
	var got float64
	h.a.cfg.Evaluate = func(_, rates map[string]float64) []advisor.Recommendation {
		got = rates["TcpExt.ListenOverflows"]
		return nil
	}
	h.tick(false, 2)
	if got != 2 { // 30 per 15s
		t.Fatalf("rate %v", got)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- Run(ctx, h.a.cfg) }()
	h.clk.ch <- time.Time{} // second tick
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestJSONSink(t *testing.T) {
	var b bytes.Buffer
	if err := NewJSONSink(&b).Emit(Event{RuleID: "r", State: StateFiring}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"rule":"r"`) || !strings.HasSuffix(b.String(), "\n") {
		t.Fatal(b.String())
	}
}
