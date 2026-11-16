package render

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
)

// Cite is one citation of a recommendation: a registered source plus the
// rule's quote and section for it. Cite is also the JSON shape of an entry in
// a finding's "sources".
type Cite struct {
	URL          string `json:"url"`
	Quote        string `json:"quote"`
	Section      string `json:"section"`
	Project      string `json:"project"`
	Title        string `json:"title"`
	LastReviewed string `json:"last_reviewed"`
	// Rule is the id of the rule that cites it. Not part of the JSON.
	Rule string `json:"-"`
}

// Label is the human name of the source: its title, else its project, else
// the host of its URL.
func (c Cite) Label() string {
	switch {
	case c.Title != "":
		return c.Title
	case c.Project != "":
		return c.Project
	}
	if u, err := url.Parse(c.URL); err == nil && u.Host != "" {
		return u.Host
	}
	return c.URL
}

// key identifies a distinct citation: the same page with a different quote
// or section is a different reference.
func (c Cite) key() string { return c.URL + "\x00" + c.Section + "\x00" + c.Quote }

// Citations resolves a recommendation to the citations of the rule(s) behind
// it. The zero value and nil are valid: they fall back to the bare URLs in
// Recommendation.Sources.
type Citations struct {
	byRule map[string][]rules.SourceRef
	reg    map[string]rules.Source
}

// NewCitations indexes the rules' sources against the sources registry.
func NewCitations(rs []rules.Rule, srcs []rules.Source) *Citations {
	c := &Citations{byRule: map[string][]rules.SourceRef{}, reg: map[string]rules.Source{}}
	for _, r := range rs {
		c.byRule[r.ID] = r.Sources
	}
	for _, s := range srcs {
		c.reg[s.URL] = s
	}
	return c
}

func (c *Citations) cite(rule string, ref rules.SourceRef) Cite {
	ct := Cite{URL: ref.URL, Quote: strings.TrimSpace(ref.Quote), Section: ref.Section, Rule: rule}
	if c != nil {
		if s, ok := c.reg[ref.URL]; ok {
			ct.Project, ct.Title, ct.LastReviewed = s.Project, s.Title, s.LastReviewed
		}
	}
	return ct
}

// For returns the distinct citations behind r, in rule then source order. A
// merged recommendation (Contributors) yields those of every contributor.
func (c *Citations) For(r advisor.Recommendation) []Cite {
	ids := r.Contributors
	if len(ids) == 0 {
		ids = []string{r.RuleID}
	}
	var out []Cite
	seen := map[string]bool{}
	add := func(ct Cite) {
		if k := ct.key(); !seen[k] {
			seen[k] = true
			out = append(out, ct)
		}
	}
	found := false
	for _, id := range ids {
		if c == nil {
			break
		}
		refs, ok := c.byRule[id]
		if !ok {
			continue
		}
		found = true
		for _, ref := range refs {
			add(c.cite(id, ref))
		}
	}
	if !found { // unknown rule (or no index): the bare URLs
		for _, u := range r.Sources {
			add(c.cite(r.RuleID, rules.SourceRef{URL: u}))
		}
	}
	return out
}

// footnotes numbers citations in first-use order.
type footnotes struct {
	idx   map[string]int
	order []Cite
}

func newFootnotes() *footnotes { return &footnotes{idx: map[string]int{}} }

// mark registers cs and returns their numbers in ascending order.
func (f *footnotes) mark(cs []Cite) []int {
	var ns []int
	for _, c := range cs {
		n, ok := f.idx[c.key()]
		if !ok {
			f.order = append(f.order, c)
			n = len(f.order)
			f.idx[c.key()] = n
		}
		ns = append(ns, n)
	}
	// insertion sort: lists are tiny
	for i := 1; i < len(ns); i++ {
		for j := i; j > 0 && ns[j] < ns[j-1]; j-- {
			ns[j], ns[j-1] = ns[j-1], ns[j]
		}
	}
	return ns
}

// markers renders numbers as "[3]" or "[3,7]"; empty for none.
func markers(ns []int) string {
	if len(ns) == 0 {
		return ""
	}
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = strconv.Itoa(n)
	}
	return "[" + strings.Join(s, ",") + "]"
}

// wrap breaks s into lines of at most width characters (a longer word gets a
// line of its own). Whitespace in s is collapsed.
func wrap(s string, width int) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// refHead is "<title or project> — <section> (reviewed <date>)".
func (c Cite) refHead() string {
	h := c.Label()
	if c.Section != "" {
		h += " — " + c.Section
	}
	if c.LastReviewed != "" {
		h += " (reviewed " + c.LastReviewed + ")"
	}
	return h
}

// Lines renders the citation as a wrapped block: the head line starts with
// label (e.g. "[3] "); continuation lines align under it. Used by scan's
// References and by explain.
func (c Cite) Lines(label string, width int) []string {
	pad := strings.Repeat(" ", len(label))
	var out []string
	for j, l := range wrap(c.refHead(), width-len(label)) {
		if j == 0 {
			out = append(out, label+l)
		} else {
			out = append(out, pad+"  "+l)
		}
	}
	if c.Quote != "" {
		for _, l := range wrap("\""+c.Quote+"\"", width-len(pad)) {
			out = append(out, pad+l)
		}
	}
	return append(out, pad+c.URL)
}

// commentLines renders a citation for generated files: the URL (and section)
// then the quote, each line prefixed with prefix.
func (c Cite) commentLines(prefix string, withRule bool) string {
	head := "source"
	if withRule && c.Rule != "" {
		head += " (" + c.Rule + ")"
	}
	line := prefix + head + ": " + c.URL
	if c.Section != "" {
		line += " — " + c.Section
	}
	out := line + "\n"
	if c.Quote != "" {
		for _, l := range wrap("\""+c.Quote+"\"", 84) {
			out += prefix + "  " + l + "\n"
		}
	}
	return out
}
