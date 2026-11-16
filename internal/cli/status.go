package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/agent"
)

const statusEvents = 10

func newStatusCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show firing rules and trends from the running agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("state")
			sf, err := agent.ReadState(path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("no state file at %s: is the agent running? (kerneltuner agent)", path)
				}
				return fmt.Errorf("read state: %w", err)
			}
			if g.JSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(sf)
			}
			printStatus(cmd.OutOrStdout(), sf, time.Now())
			return nil
		},
	}
	cmd.Flags().String("state", defaultStatePath, "state file path")
	return cmd
}

func printStatus(w io.Writer, sf *agent.StateFile, now time.Time) {
	pf(w, "host %s, interval %s, updated %s ago\n", sf.Host, sf.Interval, round(now.Sub(sf.Updated)))
	firing := 0
	for _, r := range sf.Rules {
		if r.State != agent.StateFiring {
			continue
		}
		if firing == 0 {
			pln(w, "\nFIRING")
		}
		firing++
		name := r.ID
		if r.Target != "" {
			name += " [" + r.Target + "]"
		}
		pf(w, "  %s  for %s\n", name, round(now.Sub(r.Since)))
		if r.Current != "" || r.Proposed != "" {
			pf(w, "    current %q -> proposed %q\n", r.Current, r.Proposed)
		}
		if r.LastEvidence != "" {
			pf(w, "    evidence: %s\n", r.LastEvidence)
		}
	}
	if firing == 0 {
		pln(w, "\nno rules firing")
	}
	ev := sf.RecentEvents
	if len(ev) > statusEvents {
		ev = ev[len(ev)-statusEvents:]
	}
	if len(ev) > 0 {
		pln(w, "\nRECENT EVENTS")
		for i := len(ev) - 1; i >= 0; i-- {
			pf(w, "  %s  %-8s %s\n", ev[i].Time.Local().Format("2006-01-02 15:04:05"), ev[i].State, ev[i].Message)
		}
	}
}

func round(d time.Duration) time.Duration {
	if d < 0 {
		d = 0
	}
	if d >= time.Minute {
		return d.Round(time.Minute)
	}
	return d.Round(time.Second)
}

func pf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
func pln(w io.Writer, s string)               { _, _ = fmt.Fprintln(w, s) }
