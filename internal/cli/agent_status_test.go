package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/agent"
)

func TestRunAgentWritesStateAndStops(t *testing.T) {
	state := filepath.Join(t.TempDir(), "kt", "state.json")
	g := &globalOpts{RootDir: t.TempDir(), RulesDir: t.TempDir()}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var errw bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- runAgent(ctx, g, 20*time.Millisecond, state, agent.NewJSONSink(&errw), &errw) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop")
	}
	sf, err := agent.ReadState(state)
	if err != nil {
		t.Fatal(err)
	}
	if sf.Interval != "20ms" {
		t.Fatalf("%+v", sf)
	}
}

func TestRunAgentRejectsBadInterval(t *testing.T) {
	if err := runAgent(context.Background(), &globalOpts{}, 0, "x", nil, &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
}

func writeTestState(t *testing.T) string {
	t.Helper()
	now := time.Now()
	sf := &agent.StateFile{
		Updated: now, Interval: "15s", Host: "h1 6.8.0",
		Rules: []agent.RuleState{
			{ID: "net.core.somaxconn", State: agent.StateFiring, Since: now.Add(-5 * time.Minute), Current: "128", Proposed: "4096", LastEvidence: "TcpExt.ListenOverflows=3 (threshold 0)"},
			{ID: "old.rule", Target: "eth0", State: agent.StateResolved, Since: now},
		},
	}
	for i := 0; i < 12; i++ {
		sf.RecentEvents = append(sf.RecentEvents, agent.Event{Time: now, State: agent.StateFiring, Message: "event-" + string(rune('A'+i))})
	}
	p := filepath.Join(t.TempDir(), "state.json")
	if err := agent.WriteState(p, sf); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStatusPretty(t *testing.T) {
	p := writeTestState(t)
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"status", "--state", p})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"FIRING", "net.core.somaxconn", "for 5m0s", "proposed \"4096\"", "event-L", "event-C"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "old.rule") || strings.Contains(s, "event-B") {
		t.Errorf("resolved rule or events beyond the last 10 shown:\n%s", s)
	}
}

func TestStatusJSON(t *testing.T) {
	p := writeTestState(t)
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"status", "--json", "--state", p})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var sf agent.StateFile
	if err := json.Unmarshal(out.Bytes(), &sf); err != nil || len(sf.Rules) != 2 {
		t.Fatalf("%v %s", err, out.String())
	}
}

func TestStatusMissingState(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"status", "--state", filepath.Join(t.TempDir(), "nope.json")})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "is the agent running") {
		t.Fatalf("got %v", err)
	}
}
