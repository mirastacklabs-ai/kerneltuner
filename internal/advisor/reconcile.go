package advisor

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// mergeKinds are the kinds whose (kind, param, target) identifies one setting.
var mergeKinds = map[string]bool{"sysctl": true, "sysfs": true, "limit": true, "nic": true, "cmdline": true}

// Conflict describes several rules proposing different values for one
// setting (same kind, param and target).
type Conflict struct {
	Kind, Param, Target string
	Rules               []string // contributing rule ids, highest score first
	Values              []string // their proposals, parallel to Rules
	Chosen              string   // the value generate/apply use
	Method              string   // "max", "min" or "highest-scored"
	Winner              string   // rule that supplied Chosen (highest-scored method)
	// Resolved is true when the proposals were merged (gte: max, lte: min).
	// False means a genuine disagreement: a warning, resolved by score.
	Resolved bool
	// Kept lists "rule=value" for rules that are already satisfied (status ok,
	// compare eq) at a value different from Chosen: applying Chosen breaks what
	// they asked for. Set by Conflicts only.
	Kept []string
}

// Genuine reports whether the rules truly disagree (not Resolved by merging).
func (c Conflict) Genuine() bool { return !c.Resolved }

// Setting names the conflicting setting: "param" or "param [target]".
func (c Conflict) Setting() string {
	if c.Target != "" {
		return c.Param + " [" + c.Target + "]"
	}
	return c.Param
}

// String is a one-line description.
func (c Conflict) String() string {
	parts := make([]string, len(c.Rules))
	for i := range c.Rules {
		parts[i] = c.Rules[i] + "=" + c.Values[i]
	}
	s := fmt.Sprintf("%s: %s -> %s (%s)", c.Setting(), strings.Join(parts, ", "), c.Chosen, c.Method)
	if !c.Resolved && c.Winner != "" {
		s += fmt.Sprintf(" from %s; rules disagree", c.Winner)
	}
	if len(c.Kept) > 0 {
		s += "; already satisfied and would be broken: " + strings.Join(c.Kept, ", ")
	}
	return s
}

// Reconcile merges recommendations that target the same (kind, param,
// target) setting into one, for generate and apply. scan and json output keep
// every recommendation and do not call it.
//
// Only recommend and firing results of kinds sysctl, sysfs, limit, nic and
// cmdline are merged. For gte rules the merged proposal is the max (per
// field for multi-field values), for lte the min. For eq, set or mixed rules
// with different values the highest-scored recommendation wins and the
// Conflict is not Resolved. The merged recommendation lists every rule id in
// Contributors and keeps the position of the first member. The conflicts
// returned cover every setting whose proposals differ.
func Reconcile(recs []Recommendation) ([]Recommendation, []Conflict) {
	groups := map[string][]int{}
	var keys []string
	for i, r := range recs {
		k, ok := mergeKey(r)
		if !ok {
			continue
		}
		if _, seen := groups[k]; !seen {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], i)
	}
	merged := map[int]*Recommendation{} // first index of a group -> merged rec
	drop := map[int]bool{}
	var conflicts []Conflict
	for _, k := range keys {
		idx := groups[k]
		if len(idx) < 2 || distinctRules(recs, idx) < 2 {
			continue
		}
		m, c := mergeGroup(recs, idx)
		merged[idx[0]] = m
		for _, i := range idx[1:] {
			drop[i] = true
		}
		if c != nil {
			conflicts = append(conflicts, *c)
		}
	}
	if len(merged) == 0 {
		return recs, nil
	}
	out := make([]Recommendation, 0, len(recs))
	for i, r := range recs {
		switch {
		case drop[i]:
		case merged[i] != nil:
			out = append(out, *merged[i])
		default:
			out = append(out, r)
		}
	}
	return out, conflicts
}

// Conflicts returns the conflicts Reconcile finds in recs, plus settings where
// the only recommendation contradicts a rule that is already satisfied (status
// ok, compare eq) at another value, such as app.postgres.overcommit_memory=2
// recommended on a host where app.redis.overcommit_memory=1 is ok. Those carry
// Kept and are never Resolved. Use Genuine (or ConflictBlock) to pick the ones
// to shout about.
func Conflicts(recs []Recommendation) []Conflict {
	merged, out := Reconcile(recs)
	// Settings that already conflict, by key, and ok rules per key.
	idx := map[string]int{}
	for i, c := range out {
		idx[strings.ToUpper(c.Kind+"|"+c.Param)+"|"+c.Target] = i
	}
	type okRule struct{ rule, val string }
	oks := map[string][]okRule{}
	for _, r := range recs {
		if r.Status != StatusOK || !mergeKinds[r.Kind] || r.Param == "" || r.Proposed == "" || (r.Compare != "" && r.Compare != "eq") {
			continue
		}
		k := r.Kind + "|" + strings.ToUpper(r.Param) + "|" + r.Target
		oks[k] = append(oks[k], okRule{r.RuleID, r.Proposed})
	}
	norm := func(v string) string { return strings.Join(strings.Fields(v), " ") }
	for _, r := range merged {
		k, ok := mergeKey(r)
		if !ok || len(oks[k]) == 0 {
			continue
		}
		var kept []string
		for _, o := range oks[k] {
			if norm(o.val) != norm(r.Proposed) {
				kept = append(kept, o.rule+"="+o.val)
			}
		}
		if len(kept) == 0 {
			continue
		}
		if i, found := idx[strings.ToUpper(r.Kind+"|"+r.Param)+"|"+r.Target]; found {
			out[i].Kept = kept
			continue
		}
		out = append(out, Conflict{Kind: r.Kind, Param: r.Param, Target: r.Target,
			Rules: []string{r.RuleID}, Values: []string{r.Proposed}, Chosen: r.Proposed,
			Method: "recommend-vs-ok", Winner: r.RuleID, Kept: kept})
	}
	return out
}

// ConflictBlock formats the genuine conflicts (see Conflict.Genuine) as a
// loud multi-line block for the top of a report. It is empty when there are
// none.
func ConflictBlock(cs []Conflict) string {
	var gs []Conflict
	for _, c := range cs {
		if c.Genuine() {
			gs = append(gs, c)
		}
	}
	if len(gs) == 0 {
		return ""
	}
	var b strings.Builder
	noun := "setting has"
	if len(gs) > 1 {
		noun = "settings have"
	}
	fmt.Fprintf(&b, "CONFLICT: %d %s rules that disagree. generate and apply keep the highest-scored proposal; review before applying.\n", len(gs), noun)
	for _, c := range gs {
		fmt.Fprintf(&b, "  %s\n", c.Setting())
		for i, id := range c.Rules {
			note := ""
			if c.Values[i] == c.Chosen && c.Winner == id {
				note = " (chosen)"
			}
			fmt.Fprintf(&b, "    %s proposes %s%s\n", id, c.Values[i], note)
		}
		for _, k := range c.Kept {
			fmt.Fprintf(&b, "    %s is already satisfied and would be broken\n", strings.Replace(k, "=", " wants ", 1))
		}
	}
	return b.String()
}

func mergeKey(r Recommendation) (string, bool) {
	if (r.Status != StatusRecommend && r.Status != StatusFiring) || !mergeKinds[r.Kind] || r.Param == "" {
		return "", false
	}
	return r.Kind + "|" + strings.ToUpper(r.Param) + "|" + r.Target, true
}

func distinctRules(recs []Recommendation, idx []int) int {
	seen := map[string]bool{}
	for _, i := range idx {
		seen[recs[i].RuleID] = true
	}
	return len(seen)
}

func mergeGroup(recs []Recommendation, idx []int) (*Recommendation, *Conflict) {
	// base: highest score; ties go to the earlier recommendation.
	best := idx[0]
	for _, i := range idx[1:] {
		if recs[i].Score > recs[best].Score {
			best = i
		}
	}
	m := recs[best]
	m.Contributors = nil
	seenRule := map[string]bool{}
	var ids, vals []string
	allSame := true
	modes := map[string]bool{}
	var sources []string
	seenSrc := map[string]bool{}
	for _, i := range idx {
		r := recs[i]
		if !seenRule[r.RuleID] {
			seenRule[r.RuleID] = true
			ids = append(ids, r.RuleID)
			vals = append(vals, r.Proposed)
		}
		if strings.Join(strings.Fields(r.Proposed), " ") != strings.Join(strings.Fields(m.Proposed), " ") {
			allSame = false
		}
		mode := r.Compare
		if mode == "" {
			mode = "eq"
		}
		modes[mode] = true
		for _, s := range r.Sources {
			if !seenSrc[s] {
				seenSrc[s] = true
				sources = append(sources, s)
			}
		}
	}
	// Order contributors and values highest score first for the report.
	order := make([]int, len(ids))
	for i := range order {
		order[i] = i
	}
	score := map[string]float64{}
	for _, i := range idx {
		if recs[i].Score > score[recs[i].RuleID] {
			score[recs[i].RuleID] = recs[i].Score
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return score[ids[order[a]]] > score[ids[order[b]]] })
	sortedIDs := make([]string, len(ids))
	sortedVals := make([]string, len(ids))
	for n, o := range order {
		sortedIDs[n], sortedVals[n] = ids[o], vals[o]
	}
	m.Contributors = sortedIDs
	m.Sources = sources
	if allSame {
		m.Note = "merged from " + strings.Join(sortedIDs, ", ")
		return &m, nil
	}

	c := &Conflict{Kind: m.Kind, Param: m.Param, Target: m.Target, Rules: sortedIDs, Values: sortedVals}
	switch {
	case len(modes) == 1 && modes["gte"]:
		if v, ok := pickNumeric(sortedVals, true); ok {
			c.Chosen, c.Method, c.Resolved = v, "max", true
		}
	case len(modes) == 1 && modes["lte"]:
		if v, ok := pickNumeric(sortedVals, false); ok {
			c.Chosen, c.Method, c.Resolved = v, "min", true
		}
	}
	if !c.Resolved {
		c.Chosen, c.Method, c.Winner = m.Proposed, "highest-scored", m.RuleID
	}
	m.Proposed = c.Chosen
	m.Note = c.String()
	if !c.Resolved {
		m.Note = "CONFLICT " + m.Note
	}
	return &m, c
}

// pickNumeric picks the per-field max (or min) of whitespace-separated
// numeric values, keeping the original token text so integers stay exact.
// "infinity" and "unlimited" rank above every number.
func pickNumeric(vals []string, wantMax bool) (string, bool) {
	var rows [][]string
	for _, v := range vals {
		f := strings.Fields(v)
		if len(f) == 0 || (len(rows) > 0 && len(f) != len(rows[0])) {
			return "", false
		}
		rows = append(rows, f)
	}
	out := make([]string, len(rows[0]))
	for col := range out {
		best := rows[0][col]
		for _, row := range rows[1:] {
			c, ok := cmpToken(row[col], best)
			if !ok {
				return "", false
			}
			if (wantMax && c > 0) || (!wantMax && c < 0) {
				best = row[col]
			}
		}
		if _, ok := cmpToken(best, best); !ok {
			return "", false
		}
		out[col] = best
	}
	return strings.Join(out, " "), true
}

func isInf(s string) bool {
	switch strings.ToLower(s) {
	case "infinity", "unlimited":
		return true
	}
	return false
}

// cmpToken orders two numeric tokens; ok is false if either is not numeric.
func cmpToken(a, b string) (int, bool) {
	ai, bi := isInf(a), isInf(b)
	switch {
	case ai && bi:
		return 0, true
	case ai:
		if _, ok := numVal(b); !ok {
			return 0, false
		}
		return 1, true
	case bi:
		if _, ok := numVal(a); !ok {
			return 0, false
		}
		return -1, true
	}
	if x, ok := new(big.Int).SetString(a, 10); ok {
		if y, ok := new(big.Int).SetString(b, 10); ok {
			return x.Cmp(y), true
		}
	}
	x, ok1 := numVal(a)
	y, ok2 := numVal(b)
	if !ok1 || !ok2 {
		return 0, false
	}
	switch {
	case x < y:
		return -1, true
	case x > y:
		return 1, true
	}
	return 0, true
}

func numVal(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}
