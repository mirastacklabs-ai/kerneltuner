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
	// Compare is the rule's compare mode (eq, gte, lte, set); Reconcile uses
	// it to merge duplicates.
	Compare string
	// Targets lists every item behind a foreach_summary recommendation (Target
	// is then a range such as "cpu0-63 (64)"); apply writes each one.
	Targets []string
	// Contributors lists every rule id behind a merged recommendation; set by
	// Reconcile when several rules target the same (kind, param, target).
	Contributors []string
	// Note explains how Reconcile merged the contributors.
	Note string
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
