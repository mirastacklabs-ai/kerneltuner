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
}

// BuildPlan resolves recommendations (kinds sysctl, sysfs, limit; status
// recommend or firing) into concrete operations under root. It returns an
// error if any resolved path would escape root. (It is not named Plan because
// Go does not allow a function and a type of the same name.)
func BuildPlan(recs []advisor.Recommendation, root facts.Root) (*Plan, error) {
	p := &Plan{Root: root}
	for _, r := range recs {
		if r.Status != advisor.StatusRecommend && r.Status != advisor.StatusFiring {
			continue
		}
		var (
			op   *Op
			skip string
			err  error
		)
		switch r.Kind {
		case "sysctl":
			op, skip, err = planSysctl(r, root)
		case "sysfs":
			op, skip, err = planSysfs(r, root)
		case "limit":
			op, skip, err = planLimit(r, root)
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
		p.Ops = append(p.Ops, *op)
	}
	return p, nil
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

func planSysfs(r advisor.Recommendation, root facts.Root) (*Op, string, error) {
	if r.Param == "" || r.Proposed == "" {
		return nil, "missing param or proposed value", nil
	}
	tmpl := strings.ReplaceAll(r.Param, "{item}", r.Target)
	tmpl = strings.TrimPrefix(tmpl, "/sys/")
	tmpl = strings.TrimPrefix(tmpl, "/")
	live, err := within(root.Sys, filepath.Join(root.Sys, tmpl))
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(live); err != nil {
		return nil, "sysfs path not present: " + live, nil
	}
	return &Op{RuleID: r.RuleID, Kind: r.Kind, Param: r.Param, Target: r.Target,
		Proposed: r.Proposed, LivePath: live}, "", nil
}

func planLimit(r advisor.Recommendation, root facts.Root) (*Op, string, error) {
	var key string
	switch strings.ToLower(strings.TrimPrefix(strings.ToLower(r.Param), "limit")) {
	case "nofile":
		key = "LimitNOFILE"
	case "memlock":
		key = "LimitMEMLOCK"
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
