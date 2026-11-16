package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/render"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
)

func newExplainCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <rule-id|param>",
		Short: "Explain a parameter or rule: rationale, sources, kernel notes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := loadPipeline(g)
			if err != nil {
				return err
			}
			var matched []rules.Rule
			for _, r := range p.Rules {
				if r.ID == args[0] || r.Param == args[0] {
					matched = append(matched, r)
				}
			}
			if len(matched) == 0 {
				return fmt.Errorf("no rule or param matches %q", args[0])
			}
			values, rates, err := p.sample(cmdContext(cmd), 0)
			if err != nil {
				return err
			}
			recs := advisor.Evaluate(matched, advisor.Input{
				Facts: p.Facts, Workload: p.Workload, Values: values, Rates: rates, Root: p.Root, Ecosystem: p.Eco,
			})
			cites := render.NewCitations(matched, p.Sources)
			if g.JSON {
				return explainJSON(cmd.OutOrStdout(), matched, recs, cites)
			}
			return explainText(cmd.OutOrStdout(), matched, recs, cites)
		},
	}
	return cmd
}

type explainDoc struct {
	Entries []explainEntry              `json:"rules"`
	Results []render.JSONRecommendation `json:"results"`
}

type explainEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Param      string `json:"param"`
	Domain     string `json:"domain"`
	Pack       string `json:"pack"`
	Kernel     string `json:"kernel"`
	Recommend  string `json:"recommend"`
	Compare    string `json:"compare"`
	Risk       string `json:"risk"`
	Reboot     bool   `json:"reboot"`
	Namespaced bool   `json:"namespaced"`
	Rationale  string `json:"rationale"`
	Advice     string `json:"advice"`
	Verify     string `json:"verify"`
	// Sources are the structured citations; SourceURLs is the former
	// "sources" shape (URL strings), kept for existing consumers.
	Sources    []render.Cite `json:"sources"`
	SourceURLs []string      `json:"source_urls"`
}

func entryOf(r rules.Rule, c *render.Citations) explainEntry {
	srcs := c.For(advisor.Recommendation{RuleID: r.ID})
	if srcs == nil {
		srcs = []render.Cite{}
	}
	return explainEntry{
		ID: r.ID, Title: r.Title, Kind: r.Kind, Param: r.Param, Domain: r.Domain, Pack: r.Pack,
		Kernel: r.Applies.Kernel, Recommend: r.Recommend, Compare: r.Compare, Risk: r.Risk, Reboot: r.Reboot,
		Namespaced: r.Namespaced, Rationale: strings.TrimSpace(r.Rationale), Advice: strings.TrimSpace(r.Advice),
		Verify: r.Verify, Sources: srcs, SourceURLs: r.SourceURLs(),
	}
}

func explainJSON(w io.Writer, rs []rules.Rule, recs []advisor.Recommendation, c *render.Citations) error {
	doc := explainDoc{Entries: []explainEntry{}, Results: []render.JSONRecommendation{}}
	for _, r := range rs {
		doc.Entries = append(doc.Entries, entryOf(r, c))
	}
	for _, r := range recs {
		doc.Results = append(doc.Results, render.ToJSONWith(r, c))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func explainText(w io.Writer, rs []rules.Rule, recs []advisor.Recommendation, c *render.Citations) error {
	var b strings.Builder
	for i, r := range rs {
		e := entryOf(r, c)
		if i > 0 {
			b.WriteString("\n----\n\n")
		}
		fmt.Fprintf(&b, "%s: %s\n", e.ID, e.Title)
		fmt.Fprintf(&b, "  kind: %s   param: %s   domain: %s   pack: %s\n", e.Kind, e.Param, e.Domain, e.Pack)
		fmt.Fprintf(&b, "  risk: %s   reboot: %v   namespaced: %v\n", e.Risk, e.Reboot, e.Namespaced)
		if e.Kernel != "" {
			fmt.Fprintf(&b, "  applies to kernel: %s\n", e.Kernel)
		}
		if e.Recommend != "" {
			fmt.Fprintf(&b, "  recommend: %s (compare: %s)\n", e.Recommend, orDefault(e.Compare, "eq"))
		}
		if e.Rationale != "" {
			fmt.Fprintf(&b, "\nWhy:\n  %s\n", strings.Join(strings.Fields(e.Rationale), " "))
		}
		if e.Advice != "" {
			fmt.Fprintf(&b, "\nAdvice:\n  %s\n", e.Advice)
		}
		if e.Verify != "" {
			fmt.Fprintf(&b, "\nVerify: %s\n", e.Verify)
		}
		if len(e.Sources) > 0 {
			b.WriteString("\nSources:\n")
			for i, s := range e.Sources {
				for _, l := range s.Lines(fmt.Sprintf("  [%d] ", i+1), 100) {
					b.WriteString(l + "\n")
				}
			}
		}
		b.WriteString("\nOn this host:\n")
		n := 0
		for _, rec := range recs {
			if rec.RuleID != r.ID {
				continue
			}
			n++
			target := ""
			if rec.Target != "" {
				target = " [" + rec.Target + "]"
			}
			fmt.Fprintf(&b, "  %s%s: current=%q proposed=%q", rec.Status, target, rec.Current, rec.Proposed)
			if rec.ManagedBy != "" {
				fmt.Fprintf(&b, " managed-by=%s", rec.ManagedBy)
			}
			for _, ev := range rec.Evidence {
				fmt.Fprintf(&b, " evidence:%s=%.3g(>%.3g)", ev.Metric, ev.Value, ev.Threshold)
			}
			b.WriteString("\n")
			if rec.Status == advisor.StatusError || rec.ManagedBy != "" {
				fmt.Fprintf(&b, "    %s\n", rec.Advice)
			}
		}
		if n == 0 {
			b.WriteString("  (no result)\n")
		}
	}
	_, err := io.WriteString(w, render.CleanText(b.String()))
	return err
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
