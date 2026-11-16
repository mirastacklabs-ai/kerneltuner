package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/sourcecheck"
)

// sourcesHTTPClient is the client `sources --check` uses; tests replace it.
var sourcesHTTPClient *http.Client

// runSourcesCheck implements `sources --check`: it fetches every registered
// URL (optionally only those of one pack) and verifies the quotes rules cite
// from it. It needs no host facts and sends none. Exit code 1 if any fails.
func runSourcesCheck(cmd *cobra.Command, g *globalOpts, only string) error {
	rs, _, srcs, err := loadRules(g)
	if err != nil {
		return err
	}
	quotes := map[string][]string{}
	seen := map[string]bool{}
	for _, r := range rs {
		for _, s := range r.Sources {
			q := strings.TrimSpace(s.Quote)
			if q == "" || seen[s.URL+"\x00"+q] {
				continue
			}
			seen[s.URL+"\x00"+q] = true
			quotes[s.URL] = append(quotes[s.URL], q)
		}
	}
	var items []sourcecheck.Item
	done := map[string]bool{}
	for _, s := range srcs {
		if done[s.URL] || (only != "" && s.Pack != only && path.Base(s.Pack) != only) {
			continue
		}
		done[s.URL] = true
		items = append(items, sourcecheck.Item{URL: s.URL, Pack: s.Pack, Quotes: quotes[s.URL]})
	}
	if only != "" && len(items) == 0 {
		return fmt.Errorf("--only %q: no registered sources in that pack", only)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].URL < items[j].URL })

	results := sourcecheck.Check(cmdContext(cmd), items, sourcecheck.Options{
		Client:    sourcesHTTPClient,
		UserAgent: "kerneltuner-sources-check/" + Version,
	})
	failed := 0
	for _, r := range results {
		if r.Failed() {
			failed++
		}
	}
	w := cmd.OutOrStdout()
	if g.JSON {
		if results == nil {
			results = []sourcecheck.Result{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			line := fmt.Sprintf("%-13s %s", r.Status, r.URL)
			switch r.Status {
			case sourcecheck.Redirected:
				line += " -> " + r.FinalURL
			case sourcecheck.HTTPError, sourcecheck.Unreachable:
				line += "  (" + r.Error + ")"
			}
			_, _ = fmt.Fprintln(w, line)
			for _, q := range r.Missing {
				_, _ = fmt.Fprintf(w, "              missing quote: %q\n", q)
			}
		}
		_, _ = fmt.Fprintf(w, "%d source(s) checked: %s\n", len(results), sourcecheck.Summary(results))
	}
	if failed > 0 {
		return &ExitError{Code: 1, Msg: fmt.Sprintf("sources --check: %d of %d source(s) failed", failed, len(results))}
	}
	return nil
}
