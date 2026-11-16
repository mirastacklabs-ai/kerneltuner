package advisor

import (
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

// Status is the outcome of evaluating one rule.
type Status string

// Status values.
const (
	StatusRecommend     Status = "recommend"
	StatusOK            Status = "ok"
	StatusNotApplicable Status = "not-applicable"
	StatusManaged       Status = "managed"
	StatusHostOnly      Status = "host-only"
	StatusFiring        Status = "firing"
	StatusError         Status = "error"
)

// EvidenceHit is a metric that matched a rule's evidence condition.
type EvidenceHit struct {
	Metric           string
	Value, Threshold float64
}

// Recommendation is the result of one rule (per item for foreach rules).
type Recommendation struct {
	RuleID, Kind, Domain, Param, Target, Title string // Target = item name for foreach rules
	Current, Proposed                          string
	Status                                     Status
	Risk                                       string
	Reboot                                     bool
	Score                                      float64
	Evidence                                   []EvidenceHit
	ManagedBy                                  string // "tuned:<profile>" | "bpftune"
	Rationale, Advice, Verify                  string
	Sources                                    []string
}

// Input is everything Evaluate needs.
type Input struct {
	Facts     *facts.Facts
	Workload  *workload.Workload
	Values    map[string]float64
	Rates     map[string]float64
	Root      facts.Root
	Ecosystem rules.Ecosystem
}

// Evaluate runs rs against in and returns recommendations sorted by score,
// descending. Stub.
func Evaluate(rs []rules.Rule, in Input) []Recommendation {
	_, _ = rs, in
	return nil
}
