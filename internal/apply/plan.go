package apply

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// Managed file names written under Etc.
const (
	sysctlFile = "sysctl.d/90-kerneltuner.conf"
	dropinFile = "90-kerneltuner.conf"
)

// File kinds for merging.
const (
	fileSysctl = "sysctl"
	fileDropin = "dropin"
)

// Op is one concrete change: an optional live write plus a managed-file line.
type Op struct {
	RuleID   string `json:"rule_id"`
	Kind     string `json:"kind"`
	Param    string `json:"param"`
	Target   string `json:"target,omitempty"`
	Proposed string `json:"proposed"`

	LivePath string `json:"live_path,omitempty"` // written live (sysctl, sysfs)

	File     string `json:"file,omitempty"` // managed file (sysctl.d or systemd drop-in)
	FileKind string `json:"file_kind,omitempty"`
	FileKey  string `json:"file_key,omitempty"`
	Unit     string `json:"unit,omitempty"`

	// Rules lists every rule merged into this op when several target the same setting.
	Rules []string `json:"rules,omitempty"`
}

// Skip records a selected recommendation that cannot be applied.
type Skip struct {
	RuleID string `json:"rule_id"`
	Reason string `json:"reason"`
}

// Plan is the set of operations Apply will perform.
type Plan struct {
	Root    facts.Root `json:"-"`
	Ops     []Op       `json:"ops"`
	Skipped []Skip     `json:"skipped,omitempty"`
	// Warnings lists proposals that disagreed and how they were merged.
	Warnings []string `json:"warnings,omitempty"`
	// Blocked lists genuine rule conflicts behind planned ops. Apply refuses
	// to write while any exist; selecting one contributor with --rule leaves
	// a single recommendation and so clears them.
	Blocked []string `json:"blocked,omitempty"`
}

// BuildPlan resolves recommendations (kinds sysctl, sysfs, limit; status
// recommend or firing) into concrete operations under root. It returns an
// error if any resolved path would escape root. (It is not named Plan because
// Go does not allow a function and a type of the same name.)
func BuildPlan(recs []advisor.Recommendation, root facts.Root) (*Plan, error) {
	p := &Plan{Root: root}
	genuine := map[string]string{}
	for _, c := range advisor.Conflicts(recs) {
		if c.Genuine() {
			genuine[strings.ToLower(c.Kind+"|"+c.Param+"|"+c.Target)] = c.String()
		}
	}
	recs, conflicts := advisor.Reconcile(recs)
	for _, c := range conflicts {
		p.Warnings = append(p.Warnings, c.String())
	}
	scores := map[string]float64{} // op key -> score of the rec that produced it
	for _, r := range recs {
		if r.Status != advisor.StatusRecommend && r.Status != advisor.StatusFiring {
			continue
		}
		var (
			ops  []Op
			skip string
			err  error
		)
		switch r.Kind {
		case "sysctl":
			var op *Op
			op, skip, err = planSysctl(r, root)
			if op != nil {
				ops = []Op{*op}
			}
		case "sysfs":
			ops, skip, err = planSysfs(r, root)
		case "limit":
			var op *Op
			op, skip, err = planLimit(r, root)
			if op != nil {
				ops = []Op{*op}
			}
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", r.RuleID, err)
		}
		if skip != "" {
			p.Skipped = append(p.Skipped, Skip{RuleID: r.RuleID, Reason: skip})
			continue
		}
		for _, op := range ops {
			if err := checkOp(root, op); err != nil {
				p.Skipped = append(p.Skipped, Skip{RuleID: r.RuleID, Reason: "refused: " + err.Error()})
				continue
			}
			if len(r.Contributors) > 1 {
				op.Rules = r.Contributors
			}
			if msg, ok := genuine[strings.ToLower(r.Kind+"|"+r.Param+"|"+r.Target)]; ok {
				p.addBlocked(msg)
			}
			p.add(op, r.Score, scores)
		}
	}
	return p, nil
}

func opKey(op Op) string {
	if op.LivePath != "" {
		return op.LivePath
	}
	return op.File + "|" + op.FileKey
}

// add appends op, reconciling it with an earlier op that writes the same
// file or kernel value under another spelling: equal values merge, differing
// values keep the higher-scored one and block the apply as a conflict.
func (p *Plan) add(op Op, score float64, scores map[string]float64) {
	k := opKey(op)
	for i := range p.Ops {
		if opKey(p.Ops[i]) != k {
			continue
		}
		old := &p.Ops[i]
		if norm(old.Proposed) != norm(op.Proposed) {
			msg := fmt.Sprintf("%s: %s proposes %q, %s proposes %q", filepath.Base(k), old.RuleID, old.Proposed, op.RuleID, op.Proposed)
			p.Warnings = append(p.Warnings, msg)
			p.addBlocked(msg)
			if score > scores[k] {
				*old = op
				scores[k] = score
			}
		}
		old.Rules = appendUniq(appendUniq(old.Rules, old.RuleID), op.RuleID)
		return
	}
	scores[k] = score
	p.Ops = append(p.Ops, op)
}

func (p *Plan) addBlocked(msg string) {
	for _, b := range p.Blocked {
		if b == msg {
			return
		}
	}
	p.Blocked = append(p.Blocked, msg)
}

func appendUniq(l []string, s string) []string {
	for _, x := range l {
		if x == s {
			return l
		}
	}
	return append(l, s)
}

func planSysctl(r advisor.Recommendation, root facts.Root) (*Op, string, error) {
	if r.Param == "" || r.Proposed == "" {
		return nil, "missing param or proposed value", nil
	}
	rel := strings.ReplaceAll(r.Param, ".", "/")
	live, err := within(root.Proc, filepath.Join(root.Proc, "sys", rel))
	if err != nil {
		return nil, "", err
	}
	file, err := within(root.Etc, filepath.Join(root.Etc, sysctlFile))
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(live); err != nil {
		return nil, "parameter not present on this kernel: " + r.Param, nil
	}
	return &Op{RuleID: r.RuleID, Kind: r.Kind, Param: r.Param, Proposed: r.Proposed,
		LivePath: live, File: file, FileKind: fileSysctl, FileKey: r.Param}, "", nil
}

// planSysfs yields one op per item: Targets for a foreach_summary
// recommendation (every CPU, say), else the single Target. Items whose path
// is absent (offline CPUs) are dropped; if none exists the rule is skipped.
func planSysfs(r advisor.Recommendation, root facts.Root) ([]Op, string, error) {
	if r.Param == "" || r.Proposed == "" {
		return nil, "missing param or proposed value", nil
	}
	targets := r.Targets
	if len(targets) == 0 {
		targets = []string{r.Target}
	}
	var ops []Op
	missing := ""
	for _, t := range targets {
		tmpl := strings.ReplaceAll(r.Param, "{item}", t)
		tmpl = strings.TrimPrefix(tmpl, "/sys/")
		tmpl = strings.TrimPrefix(tmpl, "/")
		live, err := within(root.Sys, filepath.Join(root.Sys, tmpl))
		if err != nil {
			return nil, "", err
		}
		if _, err := os.Stat(live); err != nil {
			missing = live
			continue
		}
		ops = append(ops, Op{RuleID: r.RuleID, Kind: r.Kind, Param: r.Param, Target: t,
			Proposed: r.Proposed, LivePath: live})
	}
	if len(ops) == 0 {
		return nil, "sysfs path not present: " + missing, nil
	}
	return ops, "", nil
}

func planLimit(r advisor.Recommendation, root facts.Root) (*Op, string, error) {
	var key string
	switch strings.ToLower(strings.TrimPrefix(strings.ToLower(r.Param), "limit")) {
	case "nofile":
		key = "LimitNOFILE"
	case "memlock":
		key = "LimitMEMLOCK"
	case "nproc":
		key = "LimitNPROC"
	default:
		return nil, "unsupported limit: " + r.Param, nil
	}
	unit := strings.TrimSpace(r.Target)
	if unit == "" {
		return nil, "limit has no systemd unit (Target empty)", nil
	}
	if !strings.Contains(unit, ".") {
		unit += ".service"
	}
	if r.Proposed == "" {
		return nil, "missing proposed value", nil
	}
	if !facts.ValidUnitName(unit) {
		return nil, fmt.Sprintf("refused: invalid systemd unit name %q", facts.SafeText(unit)), nil
	}
	file, err := within(root.Etc, filepath.Join(root.Etc, "systemd", "system", unit+".d", dropinFile))
	if err != nil {
		return nil, "", err
	}
	return &Op{RuleID: r.RuleID, Kind: r.Kind, Param: r.Param, Target: r.Target, Proposed: r.Proposed,
		File: file, FileKind: fileDropin, FileKey: key, Unit: unit}, "", nil
}

// within returns the cleaned p, or an error if it is not inside base.
func within(base, p string) (string, error) {
	p = filepath.Clean(p)
	rel, err := filepath.Rel(filepath.Clean(base), p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == "." {
		return "", fmt.Errorf("path %q escapes root %q", p, base)
	}
	return p, nil
}
