package agent

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/metrics"
)

// State machine parameters (DESIGN.md "Agent").
const (
	FireAfter    = 2 // consecutive hit ticks before firing
	ResolveAfter = 4 // consecutive clean ticks before resolved
	ReAlertEvery = time.Hour
	MaxEvents    = 50
)

// Rule states as written to the state file and sinks.
const (
	StateFiring   = "firing"
	StateResolved = "resolved"
)

// Clock abstracts time for tests.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Config configures an Agent.
type Config struct {
	Interval  time.Duration
	StatePath string
	Host      string // short host summary stored in the state file
	Evaluate  func(values, rates map[string]float64) []advisor.Recommendation
	Sample    func() (*metrics.Snapshot, error)
	Clock     Clock // default: wall clock
	Sink      Sink  // default: DefaultSink()
	// OnError receives non-fatal errors (sample, sink, state write). Default: stderr.
	OnError func(error)
}

// Event is one state transition.
type Event struct {
	Time     time.Time `json:"time"`
	RuleID   string    `json:"rule"`
	Param    string    `json:"param"`
	Target   string    `json:"target"`
	State    string    `json:"state"`
	Current  string    `json:"current"`
	Proposed string    `json:"proposed"`
	Evidence string    `json:"evidence"`
	Message  string    `json:"message"`
}

type tracked struct {
	rec        advisor.Recommendation
	hits       int
	clean      int
	state      string // "", firing, resolved
	since      time.Time
	evidence   string
	alerted    bool // the current firing episode was announced
	lastAlert  time.Time
	resolvedAt time.Time
}

// Agent is the alert state machine plus sampling loop.
type Agent struct {
	cfg    Config
	clock  Clock
	prev   *metrics.Snapshot
	rules  map[string]*tracked
	events []Event
}

// New builds an Agent, applying defaults.
func New(cfg Config) *Agent {
	a := &Agent{cfg: cfg, clock: cfg.Clock, rules: map[string]*tracked{}}
	if a.clock == nil {
		a.clock = realClock{}
	}
	if a.cfg.Sink == nil {
		a.cfg.Sink = DefaultSink()
	}
	if a.cfg.OnError == nil {
		a.cfg.OnError = func(err error) { fmt.Fprintln(os.Stderr, "kerneltuner:", err) }
	}
	return a
}

// Run ticks immediately and then every Interval until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	a := New(cfg)
	iv := a.cfg.Interval
	if iv <= 0 {
		iv = 15 * time.Second
	}
	for {
		a.Tick()
		select {
		case <-ctx.Done():
			return nil
		case <-a.clock.After(iv):
		}
	}
}

// Tick performs one sample/evaluate/transition/write cycle.
func (a *Agent) Tick() {
	cur, err := a.cfg.Sample()
	if err != nil || cur == nil {
		if err != nil {
			a.cfg.OnError(fmt.Errorf("sample: %w", err))
		}
		return
	}
	rates := metrics.Rates(a.prev, cur)
	a.prev = cur
	recs := a.cfg.Evaluate(cur.Values, rates)
	a.step(a.clock.Now(), recs)
	if a.cfg.StatePath != "" {
		if err := WriteState(a.cfg.StatePath, a.snapshot(a.clock.Now())); err != nil {
			a.cfg.OnError(fmt.Errorf("state: %w", err))
		}
	}
}

func key(r advisor.Recommendation) string { return r.RuleID + "\x00" + r.Target }

func evidenceString(r advisor.Recommendation) string {
	parts := make([]string, 0, len(r.Evidence))
	for _, e := range r.Evidence {
		parts = append(parts, fmt.Sprintf("%s=%g (threshold %g)", e.Metric, e.Value, e.Threshold))
	}
	return strings.Join(parts, "; ")
}

func (a *Agent) step(now time.Time, recs []advisor.Recommendation) {
	hit := map[string]advisor.Recommendation{}
	for _, r := range recs {
		if len(r.Evidence) > 0 || r.Status == advisor.StatusFiring {
			hit[key(r)] = r
		}
	}
	for k, r := range hit {
		t := a.rules[k]
		if t == nil {
			t = &tracked{}
			a.rules[k] = t
		}
		t.rec, t.hits, t.clean = r, t.hits+1, 0
		t.evidence = evidenceString(r)
		if t.state == StateFiring || t.hits < FireAfter {
			continue
		}
		t.state, t.since, t.resolvedAt = StateFiring, now, time.Time{}
		t.alerted = t.lastAlert.IsZero() || now.Sub(t.lastAlert) >= ReAlertEvery
		if t.alerted {
			t.lastAlert = now
			a.emit(now, t, StateFiring)
		}
	}
	for k, t := range a.rules {
		if _, ok := hit[k]; ok {
			continue
		}
		t.hits = 0
		t.clean++
		if t.state == StateFiring && t.clean >= ResolveAfter {
			t.state, t.since, t.resolvedAt = StateResolved, now, now
			if t.alerted {
				a.emit(now, t, StateResolved)
			}
		}
		// forget quiet entries once the re-alert window has passed
		if t.state != StateFiring && t.hits == 0 && now.Sub(t.lastAlert) >= ReAlertEvery &&
			(t.resolvedAt.IsZero() || now.Sub(t.resolvedAt) >= ReAlertEvery) {
			delete(a.rules, k)
		}
	}
}

func (a *Agent) emit(now time.Time, t *tracked, state string) {
	r := t.rec
	ev := Event{
		Time: now, RuleID: r.RuleID, Param: r.Param, Target: r.Target, State: state,
		Current: r.Current, Proposed: r.Proposed, Evidence: t.evidence,
	}
	target := ""
	if r.Target != "" {
		target = " [" + r.Target + "]"
	}
	ev.Message = fmt.Sprintf("%s%s %s: %s", r.RuleID, target, state, r.Title)
	a.events = append(a.events, ev)
	if len(a.events) > MaxEvents {
		a.events = a.events[len(a.events)-MaxEvents:]
	}
	if err := a.cfg.Sink.Emit(ev); err != nil {
		a.cfg.OnError(fmt.Errorf("sink: %w", err))
	}
}

func (a *Agent) snapshot(now time.Time) *StateFile {
	sf := &StateFile{
		Updated:      now,
		Interval:     a.cfg.Interval.String(),
		Host:         a.cfg.Host,
		Rules:        []RuleState{},
		RecentEvents: append([]Event{}, a.events...),
	}
	for _, t := range a.rules {
		if t.state == "" {
			continue
		}
		sf.Rules = append(sf.Rules, RuleState{
			ID: t.rec.RuleID, Target: t.rec.Target, State: t.state, Since: t.since,
			LastEvidence: t.evidence, Current: t.rec.Current, Proposed: t.rec.Proposed,
		})
	}
	sort.Slice(sf.Rules, func(i, j int) bool {
		if sf.Rules[i].ID != sf.Rules[j].ID {
			return sf.Rules[i].ID < sf.Rules[j].ID
		}
		return sf.Rules[i].Target < sf.Rules[j].Target
	})
	return sf
}
