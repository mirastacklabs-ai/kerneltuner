package rules

// Rule is one entry of a pack YAML file. See docs/DESIGN.md "Schema".
type Rule struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Kind    string `yaml:"kind"` // sysctl|sysfs|limit|nic|cmdline|service|runtime|report|antipattern
	Param   string `yaml:"param"`
	Domain  string `yaml:"domain"` // network|limits|memory|cpu|storage|ai
	Foreach string `yaml:"foreach"`
	// ForeachSummary collapses foreach items with identical results into one
	// recommendation whose Target is a range such as "cpu0-63 (64)".
	ForeachSummary bool `yaml:"foreach_summary"`
	// RequireEvidence makes recommend/firing need at least one evidence hit;
	// otherwise the status is ok.
	RequireEvidence bool       `yaml:"require_evidence"`
	Filter          string     `yaml:"filter"`
	Applies         Applies    `yaml:"applies"`
	Recommend       string     `yaml:"recommend"`
	Compare         string     `yaml:"compare"` // eq|gte|lte|set
	Detect          string     `yaml:"detect"`
	Evidence        []Evidence `yaml:"evidence"`
	Namespaced      bool       `yaml:"namespaced"`
	Reboot          bool       `yaml:"reboot"`
	Risk            string     `yaml:"risk"` // low|medium|high
	Rationale       string     `yaml:"rationale"`
	Advice          string     `yaml:"advice"`
	Verify          string     `yaml:"verify"`
	Sources         []string   `yaml:"sources"`
	Tests           []RuleTest `yaml:"tests"`

	// Set by the loader, not from YAML.
	Pack string `yaml:"-"` // e.g. "kernel/network"
}

// Applies holds applicability conditions; all present conditions must hold.
type Applies struct {
	Kernel string `yaml:"kernel"` // e.g. ">=4.9 <6.18"
	When   string `yaml:"when"`   // expr -> bool
}

// Evidence is a metric condition that raises a rule's score.
type Evidence struct {
	Metric  string   `yaml:"metric"`
	RateGT  *float64 `yaml:"rate_gt"`
	ValueGT *float64 `yaml:"value_gt"`
	RatioGT *float64 `yaml:"ratio_gt"`
	Of      string   `yaml:"of"`
}

// RuleTest is a golden test: evaluate the rule against a fixture.
type RuleTest struct {
	Fixture string `yaml:"fixture"`
	Expect  Expect `yaml:"expect"`
}

// Expect is the expected outcome of a RuleTest.
type Expect struct {
	Status   string `yaml:"status"`
	Proposed string `yaml:"proposed"`
	Target   string `yaml:"target"`
}

// Source is one entry of a pack's sources.yaml.
type Source struct {
	URL          string `yaml:"url"`
	Project      string `yaml:"project"`
	Covers       string `yaml:"covers"`
	LastReviewed string `yaml:"last_reviewed"`
}

// TunedProfile lists the sysctls a tuned profile sets.
type TunedProfile struct {
	Sysctl map[string]string `yaml:"sysctl"`
}

// Ecosystem is the data from rules/ecosystem/{tuned,bpftune}.yaml.
type Ecosystem struct {
	Tuned   map[string]TunedProfile // profile name -> sysctls (tuned.yaml)
	Bpftune []string                // sysctls bpftune tunes (bpftune.yaml)
}
