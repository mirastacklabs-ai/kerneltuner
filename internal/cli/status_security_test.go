package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/agent"
)

// R-Q04: status output has no control characters.
func TestStatusStripsControlCharacters(t *testing.T) {
	now := time.Now()
	sf := &agent.StateFile{Updated: now, Interval: "10s\x1b", Host: "h\x1b[2Jost",
		Rules:        []agent.RuleState{{ID: "r\x1b", Target: "t\n", State: agent.StateFiring, Since: now, Current: "c\x07", Proposed: "p\x9b", LastEvidence: "e\r"}},
		RecentEvents: []agent.Event{{Time: now, State: "firing", Message: "m\x1b]0;x\x07"}}}
	var out bytes.Buffer
	printStatus(&out, sf, now)
	for _, r := range out.String() {
		if r != '\n' && (r < 0x20 || (r >= 0x7f && r <= 0x9f)) {
			t.Fatalf("control character %q in %q", r, out.String())
		}
	}
}

// R-Q08: a huge state file is refused.
func TestStatusRejectsHugeState(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	big := `{"host":"` + strings.Repeat("a", 5<<20) + `"}`
	if err := os.WriteFile(p, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.ReadState(p); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v", err)
	}
	small := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(small, []byte(`{"host":"ok"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if sf, err := agent.ReadState(small); err != nil || sf.Host != "ok" {
		t.Fatalf("%v %v", sf, err)
	}
}
