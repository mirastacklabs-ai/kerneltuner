package advisor

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

var riskWeight = map[string]float64{"low": 1.0, "medium": 0.7, "high": 0.4}

// Evaluate runs rs against in and returns recommendations sorted by score,
// descending. A rule that fails (bad expression, panic) yields a single
// recommendation with status error; Evaluate itself never panics.
func Evaluate(rs []rules.Rule, in Input) []Recommendation {
	if in.Facts == nil {
		in.Facts = &facts.Facts{}
	}
	if in.Workload == nil {
		in.Workload = &workload.Workload{}
	}
	var out []Recommendation
	for i := range rs {
		out = append(out, evalRule(&rs[i], in)...)
	}
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.Score != y.Score {
			return x.Score > y.Score
		}
		if x.Domain != y.Domain {
			return x.Domain < y.Domain
		}
		if x.RuleID != y.RuleID {
			return x.RuleID < y.RuleID
		}
		return x.Target < y.Target
	})
	return out
}

func base(r *rules.Rule) Recommendation {
	return Recommendation{
		RuleID: r.ID, Kind: r.Kind, Domain: r.Domain, Param: r.Param, Title: r.Title,
		Risk: r.Risk, Reboot: r.Reboot, Rationale: r.Rationale, Verify: r.Verify,
		Sources: r.Sources, Status: StatusNotApplicable,
	}
}

func errRec(r *rules.Rule, target string, err error) Recommendation {
	rec := base(r)
	rec.Target = target
	rec.Status = StatusError
	rec.Advice = "error: " + err.Error()
	return rec
}

type item struct {
	name string
	val  any
}

func evalRule(r *rules.Rule, in Input) (recs []Recommendation) {
	defer func() {
		if p := recover(); p != nil {
			recs = []Recommendation{errRec(r, "", fmt.Errorf("panic: %v", p))}
		}
	}()
	ein := rules.EnvInput{Facts: in.Facts, Workload: in.Workload, Values: in.Values, Rates: in.Rates, Root: in.Root}

	if r.Applies.Kernel != "" {
		kc, err := rules.ParseKernelConstraint(r.Applies.Kernel)
		if err != nil {
			return []Recommendation{errRec(r, "", err)}
		}
		k := in.Facts.Kernel
		if (k.Major == 0 && k.Minor == 0) || !kc.Match(k.Major, k.Minor, k.Patch) {
			return []Recommendation{base(r)}
		}
	}
	if r.Applies.When != "" {
		ok, err := rules.EvalBool(r.Applies.When, rules.NewEnv(ein, nil, nil))
		if err != nil {
			return []Recommendation{errRec(r, "", fmt.Errorf("applies.when: %w", err))}
		}
		if !ok {
			return []Recommendation{base(r)}
		}
	}

	items := []item{{}}
	if r.Foreach != "" {
		items = items[:0]
		for _, it := range itemsOf(r.Foreach, in) {
			if r.Filter != "" {
				ok, err := rules.EvalBool(r.Filter, rules.NewEnv(ein, nil, it.val))
				if err != nil {
					return []Recommendation{errRec(r, it.name, fmt.Errorf("filter: %w", err))}
				}
				if !ok {
					continue
				}
			}
			items = append(items, it)
		}
		if len(items) == 0 {
			return []Recommendation{base(r)}
		}
	}
	for _, it := range items {
		rec, err := evalItem(r, in, ein, it)
		if err != nil {
			rec = errRec(r, it.name, err)
		}
		recs = append(recs, rec)
	}
	return recs
}

func itemsOf(kind string, in Input) []item {
	var out []item
	switch kind {
	case "nic":
		for _, n := range in.Facts.NICs {
			out = append(out, item{n.Name, n})
		}
	case "block":
		for _, b := range in.Facts.Block {
			out = append(out, item{b.Name, b})
		}
	case "accel":
		for _, a := range in.Facts.Accel {
			out = append(out, item{a.PCIAddr, a})
		}
	case "rdma":
		for _, d := range in.Facts.RDMA {
			out = append(out, item{d.Name, d})
		}
	case "app":
		for _, a := range in.Workload.Apps {
			name := a.Name
			if a.Unit != "" {
				name = a.Unit // limit rules target the systemd unit
			}
			out = append(out, item{name, a})
		}
	case "numa":
		for _, n := range in.Facts.NUMA {
			out = append(out, item{strconv.Itoa(n.ID), n})
		}
	}
	return out
}

func evalItem(r *rules.Rule, in Input, ein rules.EnvInput, it item) (Recommendation, error) {
	rec := base(r)
	rec.Target = it.name
	env := rules.NewEnv(ein, nil, it.val)
	rec.Evidence = evidenceHits(r, in, it.name)

	finish := func() Recommendation {
		rec.Advice = renderAdvice(r.Advice, in, rec, it.val, env.Current)
		if rec.Status == StatusRecommend || rec.Status == StatusFiring {
			hits := float64(len(rec.Evidence))
			rec.Score = riskWeight[r.Risk] * (1 + 2*hits)
			if r.Kind == "antipattern" {
				rec.Score *= 1.5
			}
		}
		return rec
	}

	switch r.Kind {
	case "antipattern", "report":
		fire, err := rules.EvalBool(r.Detect, env)
		if err != nil {
			return rec, fmt.Errorf("detect: %w", err)
		}
		rec.Status = StatusOK
		if fire {
			rec.Status = StatusFiring
		}
		return finish(), nil
	case "runtime":
		rec.Status = StatusRecommend
		return finish(), nil
	}

	display, cur, ok, err := currentValue(r, in, it)
	if err != nil {
		return rec, err
	}
	if !ok {
		return finish(), nil // not-applicable: nothing to read on this host
	}
	rec.Current = display
	env.Current = cur

	var proposed any = ""
	if r.Recommend != "" {
		if proposed, err = rules.Eval(r.Recommend, env); err != nil {
			return rec, fmt.Errorf("recommend: %w", err)
		}
	}
	prop := normalize(proposed, r.Kind == "limit")
	rec.Proposed = formatValue(prop)
	if r.Kind == "limit" && isInfinity(prop) {
		rec.Proposed = "infinity"
	}

	if matches(r, cur, prop, display) {
		rec.Status = StatusOK
		return finish(), nil
	}
	rec.Status = StatusRecommend
	switch {
	case in.Facts.Host.Container && !r.Namespaced && r.Kind != "service":
		rec.Status = StatusHostOnly
	case r.Kind == "sysctl":
		if by, advice := managedBy(in, r.Param); by != "" {
			rec.Status, rec.ManagedBy = StatusManaged, by
			rec.Advice = advice
			return rec, nil
		}
	}
	return finish(), nil
}

// managedBy reports whether tuned or bpftune already owns a sysctl.
func managedBy(in Input, param string) (by, advice string) {
	if t := in.Facts.Tuned; t.Active && t.Profile != "" {
		if v, ok := in.Ecosystem.Tuned[t.Profile].Sysctl[param]; ok {
			return "tuned:" + t.Profile, fmt.Sprintf("tuned profile %q sets %s=%s; change the profile instead of the sysctl", t.Profile, param, v)
		}
	}
	if in.Facts.Bpftune {
		for _, p := range in.Ecosystem.Bpftune {
			if p == param {
				return "bpftune", "writing this sysctl disables the bpftune tuner"
			}
		}
	}
	return "", ""
}

// currentValue reads the live value. ok=false means it is unavailable here
// (status not-applicable).
func currentValue(r *rules.Rule, in Input, it item) (display string, typed any, ok bool, err error) {
	switch r.Kind {
	case "sysctl":
		v, found := sysctlValue(in, r.Param)
		if !found {
			return "", nil, false, nil
		}
		return v, rules.TypedValue(v), true, nil
	case "sysfs":
		p := strings.ReplaceAll(r.Param, "{item}", it.name)
		full, good := rules.ResolveUnder(in.Root, p)
		if !good || !strings.HasPrefix(filepath.Clean(p), "/sys/") {
			return "", nil, false, fmt.Errorf("sysfs path %q is not under /sys", p)
		}
		b, rerr := os.ReadFile(full)
		if rerr != nil {
			return "", nil, false, nil
		}
		v := strings.Join(strings.Fields(unbracket(string(b))), " ")
		return v, rules.TypedValue(v), true, nil
	case "cmdline":
		v, found := in.Facts.Kernel.CmdlineArgs[r.Param]
		if !found {
			return "", "", true, nil
		}
		if v == "" {
			return r.Param, "", true, nil // bare flag present
		}
		return v, rules.TypedValue(v), true, nil
	case "service":
		state := "stopped"
		if serviceRunning(in, r.Param) {
			state = "running"
		}
		return state, state, true, nil
	case "limit":
		app, isApp := it.val.(workload.App)
		if !isApp {
			return "", nil, false, fmt.Errorf("kind limit needs foreach: app")
		}
		var v uint64
		switch strings.ToUpper(r.Param) {
		case "LIMITNOFILE":
			v = app.NoFileSoft
		case "LIMITMEMLOCK":
			v = app.MemlockSoft
		default:
			return "", nil, false, fmt.Errorf("unknown limit %q", r.Param)
		}
		if v == math.MaxUint64 {
			return "unlimited", math.MaxUint64 + 0.0, true, nil
		}
		return strconv.FormatUint(v, 10), float64(v), true, nil
	case "nic":
		n, isNIC := it.val.(facts.NIC)
		if !isNIC {
			return "", nil, false, fmt.Errorf("kind nic needs foreach: nic")
		}
		var v int
		switch r.Param {
		case "combined":
			v = n.Combined
		case "rx_ring":
			v = n.RxRing
		case "tx_ring":
			v = n.TxRing
		default:
			return "", nil, false, fmt.Errorf("unknown nic param %q", r.Param)
		}
		if v == 0 {
			return "", nil, false, nil
		}
		return strconv.Itoa(v), float64(v), true, nil
	}
	return "", nil, false, fmt.Errorf("unsupported kind %q", r.Kind)
}

func sysctlValue(in Input, name string) (string, bool) {
	if v, ok := in.Facts.Sysctl[name]; ok {
		return v, true
	}
	if in.Root.Proc == "" || strings.ContainsAny(name, "/") || strings.Contains(name, "..") {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(in.Root.Proc, "sys", strings.ReplaceAll(name, ".", "/")))
	if err != nil {
		return "", false
	}
	return strings.Join(strings.Fields(string(b)), " "), true
}

// unbracket turns "always [madvise] never" into "madvise".
func unbracket(s string) string {
	if i := strings.IndexByte(s, '['); i >= 0 {
		if j := strings.IndexByte(s[i:], ']'); j > 0 {
			return s[i+1 : i+j]
		}
	}
	return s
}

func serviceRunning(in Input, unit string) bool {
	name := strings.TrimSuffix(unit, ".service")
	switch name {
	case "irqbalance":
		if in.Facts.Irqbalance {
			return true
		}
	case "bpftune":
		if in.Facts.Bpftune {
			return true
		}
	case "tuned":
		if in.Facts.Tuned.Active {
			return true
		}
	}
	for _, p := range in.Facts.Procs {
		if p.Comm == name || p.Unit == unit || p.Unit == name+".service" {
			return true
		}
	}
	return false
}

// normalize converts an expression result to float64, []float64 or string.
func normalize(v any, limit bool) any {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		t := rules.TypedValue(x)
		if s, ok := t.(string); ok && limit {
			if l := strings.ToLower(s); l == "unlimited" || l == "infinity" {
				return math.MaxUint64 + 0.0
			}
		}
		return t
	case []any:
		out := make([]float64, 0, len(x))
		parts := make([]string, 0, len(x))
		allNum := true
		for _, e := range x {
			f, ok := toFloat(e)
			if !ok {
				allNum = false
			}
			out = append(out, f)
			parts = append(parts, fmt.Sprint(e))
		}
		if !allNum {
			return strings.Join(parts, " ")
		}
		if len(out) == 1 {
			return out[0]
		}
		return out
	case []float64:
		if len(x) == 1 {
			return x[0]
		}
		return x
	}
	if f, ok := toFloat(v); ok {
		return f
	}
	return fmt.Sprint(v)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func isInfinity(v any) bool {
	f, ok := v.(float64)
	return ok && f >= math.MaxUint64
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatValue(v any) string {
	switch x := v.(type) {
	case float64:
		return formatFloat(x)
	case []float64:
		parts := make([]string, len(x))
		for i, f := range x {
			parts[i] = formatFloat(f)
		}
		return strings.Join(parts, " ")
	case string:
		return x
	}
	return fmt.Sprint(v)
}

// matches decides ok vs recommend per the rule's compare mode.
func matches(r *rules.Rule, cur, prop any, display string) bool {
	mode := r.Compare
	if mode == "" {
		mode = "eq"
	}
	if r.Kind == "cmdline" {
		if display == "" {
			return false // key absent
		}
		if prop == "" || mode == "set" {
			return true // flag, or any value accepted
		}
	}
	if mode == "set" {
		return display != ""
	}
	cn, cok := asNums(cur)
	pn, pok := asNums(prop)
	if cok && pok {
		if len(cn) != len(pn) {
			return false
		}
		for i := range cn {
			switch mode {
			case "gte":
				if cn[i] < pn[i] {
					return false
				}
			case "lte":
				if cn[i] > pn[i] {
					return false
				}
			default:
				if math.Abs(cn[i]-pn[i]) > 1e-9 {
					return false
				}
			}
		}
		return true
	}
	if cok != pok {
		return false
	}
	return formatValue(cur) == formatValue(prop)
}

func asNums(v any) ([]float64, bool) {
	switch x := v.(type) {
	case float64:
		return []float64{x}, true
	case []float64:
		return x, true
	}
	return nil, false
}

func evidenceHits(r *rules.Rule, in Input, target string) []EvidenceHit {
	var hits []EvidenceHit
	sub := func(s string) string { return strings.ReplaceAll(s, "{item}", target) }
	for _, e := range r.Evidence {
		m := sub(e.Metric)
		switch {
		case e.RateGT != nil:
			if v := in.Rates[m]; v > *e.RateGT {
				hits = append(hits, EvidenceHit{m, v, *e.RateGT})
			}
		case e.ValueGT != nil:
			if v := in.Values[m]; v > *e.ValueGT {
				hits = append(hits, EvidenceHit{m, v, *e.ValueGT})
			}
		case e.RatioGT != nil:
			den := in.Values[sub(e.Of)]
			if den <= 0 {
				continue
			}
			if v := in.Values[m] / den; v > *e.RatioGT {
				hits = append(hits, EvidenceHit{m, v, *e.RatioGT})
			}
		}
	}
	return hits
}

func renderAdvice(text string, in Input, rec Recommendation, item, current any) string {
	if text == "" {
		return ""
	}
	t, err := template.New("advice").Parse(text)
	if err != nil {
		return text
	}
	var buf bytes.Buffer
	data := map[string]any{
		"item": item, "facts": in.Facts, "workload": in.Workload,
		"current": current, "proposed": rec.Proposed, "target": rec.Target,
	}
	if err := t.Execute(&buf, data); err != nil {
		return text
	}
	return buf.String()
}
