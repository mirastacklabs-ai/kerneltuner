package agent

import (
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/coreos/go-systemd/v22/journal"
)

// Sink receives state transitions.
type Sink interface {
	Emit(Event) error
}

// DefaultSink is journald when available, otherwise JSON lines on stderr.
func DefaultSink() Sink {
	if journal.Enabled() {
		return JournalSink{}
	}
	return NewJSONSink(os.Stderr)
}

// JournalSink writes structured entries to journald.
type JournalSink struct{}

// Emit sends one entry: priority WARNING for firing, INFO for resolved.
func (JournalSink) Emit(e Event) error {
	pri := journal.PriInfo
	if e.State == StateFiring {
		pri = journal.PriWarning
	}
	return journal.Send(e.Message, pri, map[string]string{
		"SYSLOG_IDENTIFIER": "kerneltuner",
		"KT_RULE":           e.RuleID,
		"KT_PARAM":          e.Param,
		"KT_TARGET":         e.Target,
		"KT_CURRENT":        e.Current,
		"KT_PROPOSED":       e.Proposed,
		"KT_EVIDENCE":       e.Evidence,
		"KT_STATE":          e.State,
	})
}

// JSONSink writes one JSON object per line.
type JSONSink struct {
	mu sync.Mutex
	w  io.Writer
}

// NewJSONSink returns a JSON-lines sink writing to w.
func NewJSONSink(w io.Writer) *JSONSink { return &JSONSink{w: w} }

// Emit writes e as one line.
func (s *JSONSink) Emit(e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.w.Write(append(b, '\n'))
	return err
}
