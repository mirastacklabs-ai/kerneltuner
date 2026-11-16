package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
)

// Enumerations accepted in rule files.
var (
	Kinds     = []string{"sysctl", "sysfs", "limit", "nic", "cmdline", "service", "runtime", "report", "antipattern"}
	Domains   = []string{"network", "limits", "memory", "cpu", "storage", "ai"}
	Risks     = []string{"low", "medium", "high"}
	Compares  = []string{"eq", "gte", "lte", "set"}
	Foreaches = []string{"nic", "block", "accel", "rdma", "app", "numa"}
	Statuses  = []string{"recommend", "ok", "not-applicable", "managed", "host-only", "firing", "error"}
	// NICParams are the valid params of kind nic.
	NICParams = []string{"combined", "rx_ring", "tx_ring"}
	// LimitParams are the valid params of kind limit.
	LimitParams = []string{"LimitNOFILE", "LimitMEMLOCK"}
)

var (
	idRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	sysctlRe = regexp.MustCompile(`^[a-z0-9_-]+(\.[a-z0-9_-]+)+$`)
)

func in(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// Validate checks rs against the schema and returns every problem found
// (not just the first). Fixture existence is checked under fixture.FixturesDir;
// if that directory cannot be located, that is itself reported.
func Validate(rs []Rule, srcs []Source) []error {
	dir, err := fixture.FixturesDir()
	if err != nil {
		return append(ValidateWith(rs, srcs, ""), err)
	}
	return ValidateWith(rs, srcs, dir)
}

// ValidateWith is Validate with an explicit fixtures directory. An empty
// fixturesDir skips the fixture-existence check.
func ValidateWith(rs []Rule, srcs []Source, fixturesDir string) []error {
	var errs []error
	registered := map[string]bool{}
	for _, s := range srcs {
		registered[s.URL] = true
	}
	seen := map[string]string{}
	for i := range rs {
		r := &rs[i]
		where := r.ID
		if where == "" {
			where = fmt.Sprintf("rule #%d (%s)", i+1, r.Pack)
		}
		add := func(format string, a ...any) {
			errs = append(errs, fmt.Errorf("%s: %s", where, fmt.Sprintf(format, a...)))
		}
		validateRule(r, registered, fixturesDir, add)
		if r.ID != "" {
			if prev, dup := seen[r.ID]; dup {
				add("duplicate id (also in %s)", prev)
			} else {
				seen[r.ID] = r.Pack
			}
		}
	}
	return errs
}

func validateRule(r *Rule, registered map[string]bool, fixturesDir string, add func(string, ...any)) {
	if !idRe.MatchString(r.ID) {
		add("id %q must match %s", r.ID, idRe)
	}
	if r.Title == "" {
		add("title is required")
	}
	if !in(Kinds, r.Kind) {
		add("kind %q must be one of %v", r.Kind, Kinds)
	}
	if !in(Domains, r.Domain) {
		add("domain %q must be one of %v", r.Domain, Domains)
	}
	if !in(Risks, r.Risk) {
		add("risk %q must be one of %v", r.Risk, Risks)
	}
	if r.Compare != "" && !in(Compares, r.Compare) {
		add("compare %q must be one of %v", r.Compare, Compares)
	}
	if r.Foreach != "" && !in(Foreaches, r.Foreach) {
		add("foreach %q must be one of %v", r.Foreach, Foreaches)
	}
	if r.Filter != "" && r.Foreach == "" {
		add("filter requires foreach")
	}

	// Required fields per kind.
	switch r.Kind {
	case "sysctl", "sysfs", "limit", "nic", "service":
		need(r.Param != "", add, "param is required for kind %s", r.Kind)
		need(r.Recommend != "", add, "recommend is required for kind %s", r.Kind)
	case "cmdline":
		need(r.Param != "", add, "param (the cmdline key) is required for kind cmdline")
		need(r.Reboot, add, "kind cmdline requires reboot: true")
	case "runtime":
		need(r.Advice != "", add, "advice is required for kind runtime")
	case "antipattern", "report":
		need(r.Detect != "", add, "detect is required for kind %s", r.Kind)
	}
	switch r.Kind {
	case "sysctl":
		if r.Param != "" && !sysctlRe.MatchString(r.Param) {
			add("sysctl param %q must be a dotted name", r.Param)
		}
	case "sysfs":
		if !strings.HasPrefix(r.Param, "/sys/") || strings.Contains(r.Param, "..") {
			add("sysfs param %q must be an absolute /sys/... path without ..", r.Param)
		}
	case "limit":
		if !in(LimitParams, r.Param) {
			add("limit param %q must be one of %v", r.Param, LimitParams)
		}
		if r.Foreach != "app" {
			add("kind limit requires foreach: app")
		}
	case "nic":
		if !in(NICParams, r.Param) {
			add("nic param %q must be one of %v", r.Param, NICParams)
		}
		if r.Foreach != "nic" {
			add("kind nic requires foreach: nic")
		}
	}

	// Expressions.
	for _, e := range []struct{ name, src string }{
		{"applies.when", r.Applies.When},
		{"filter", r.Filter},
		{"recommend", r.Recommend},
		{"detect", r.Detect},
	} {
		if e.src == "" {
			continue
		}
		if _, err := Compile(e.src); err != nil {
			add("%s does not compile: %v", e.name, err)
		}
	}
	if r.Applies.Kernel != "" {
		if _, err := ParseKernelConstraint(r.Applies.Kernel); err != nil {
			add("applies.kernel: %v", err)
		}
	}
	if r.Advice != "" {
		if _, err := template.New("advice").Parse(r.Advice); err != nil {
			add("advice template: %v", err)
		}
	}
	for j, ev := range r.Evidence {
		n := 0
		for _, set := range []bool{ev.RateGT != nil, ev.ValueGT != nil, ev.RatioGT != nil} {
			if set {
				n++
			}
		}
		if ev.Metric == "" {
			add("evidence[%d]: metric is required", j)
		}
		if n != 1 {
			add("evidence[%d]: exactly one of rate_gt, value_gt, ratio_gt is required", j)
		}
		if (ev.RatioGT != nil) != (ev.Of != "") {
			add("evidence[%d]: ratio_gt and of go together", j)
		}
	}

	// Sources.
	if len(r.Sources) == 0 {
		add("at least one source URL is required")
	}
	for _, u := range r.Sources {
		if !registered[u] {
			add("source %q is not registered in any sources.yaml", u)
		}
	}

	// Tests.
	if len(r.Tests) == 0 {
		add("at least one golden test is required")
	}
	for j, t := range r.Tests {
		if t.Fixture == "" {
			add("tests[%d]: fixture is required", j)
		} else if fixturesDir != "" {
			if _, err := fixture.ParseIn(fixturesDir, t.Fixture); err != nil {
				add("tests[%d]: %v", j, err)
			} else if _, err := os.Stat(filepath.Join(fixturesDir, t.Fixture, "fixture.yaml")); err != nil {
				add("tests[%d]: fixture %q does not exist", j, t.Fixture)
			}
		}
		if !in(Statuses, t.Expect.Status) {
			add("tests[%d]: expect.status %q must be one of %v", j, t.Expect.Status, Statuses)
		}
	}
}

func need(ok bool, add func(string, ...any), format string, a ...any) {
	if !ok {
		add(format, a...)
	}
}
