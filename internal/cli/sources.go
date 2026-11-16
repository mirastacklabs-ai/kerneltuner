package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
)

type sourceOut struct {
	URL          string `json:"url"`
	Project      string `json:"project"`
	Title        string `json:"title"`
	Covers       string `json:"covers"`
	LastReviewed string `json:"last_reviewed"`
}

func newSourcesCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "List upstream docs behind the active recommendations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if check, _ := cmd.Flags().GetBool("check"); check {
				only, _ := cmd.Flags().GetString("only")
				return runSourcesCheck(cmd, g, only)
			}
			all, _ := cmd.Flags().GetBool("all")
			p, err := loadPipeline(g)
			if err != nil {
				return err
			}
			registry := map[string]rules.Source{}
			for _, s := range p.Sources {
				registry[s.URL] = s
			}
			var urls []string
			if all {
				for u := range registry {
					urls = append(urls, u)
				}
			} else {
				values, rates, err := p.sample(cmdContext(cmd), 0)
				if err != nil {
					return err
				}
				seen := map[string]bool{}
				for _, r := range p.evaluate(values, rates) {
					if r.Status != advisor.StatusRecommend && r.Status != advisor.StatusFiring {
						continue
					}
					for _, u := range r.Sources {
						if !seen[u] {
							seen[u] = true
							urls = append(urls, u)
						}
					}
				}
			}
			sort.Strings(urls)
			out := make([]sourceOut, 0, len(urls))
			for _, u := range urls {
				s := registry[u]
				out = append(out, sourceOut{URL: u, Project: s.Project, Title: s.Title, Covers: s.Covers, LastReviewed: s.LastReviewed})
			}
			w := cmd.OutOrStdout()
			if g.JSON {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			if len(out) == 0 {
				_, err := fmt.Fprintln(w, "no sources to list")
				return err
			}
			for _, s := range out {
				line := s.URL
				var meta []string
				if s.Title != "" {
					meta = append(meta, s.Title)
				}
				if s.Project != "" {
					meta = append(meta, s.Project)
				}
				if s.Covers != "" {
					meta = append(meta, s.Covers)
				}
				if s.LastReviewed != "" {
					meta = append(meta, "reviewed "+s.LastReviewed)
				}
				if len(meta) > 0 {
					line += "  (" + strings.Join(meta, "; ") + ")"
				}
				if _, err := fmt.Fprintln(w, line); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("check", false, "opt-in network check: fetch every registered URL and verify it still contains the quotes rules cite (exit 1 on any failure)")
	cmd.Flags().String("only", "", "with --check: only sources registered in this pack (e.g. kernel/network or network)")
	cmd.Flags().Bool("all", false, "list every registered source, not only those behind active recommendations")
	return cmd
}
