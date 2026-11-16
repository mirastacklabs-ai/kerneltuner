package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/agent"
	"github.com/mirastacklabs-ai/kerneltuner/internal/metrics"
)

const (
	defaultStatePath = "/run/kerneltuner/state.json"
	factsRefresh     = 10 * time.Minute
)

func newAgentCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the observe-only sampling daemon that alerts via journald",
		Long: "Samples kernel counters every --interval, evaluates the rules and logs " +
			"firing/resolved transitions to journald (JSON lines on stderr without journald). " +
			"The only file it writes is the state file.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			interval, _ := cmd.Flags().GetDuration("interval")
			state, _ := cmd.Flags().GetString("state")
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			return runAgent(ctx, g, interval, state, nil, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().Duration("interval", 15*time.Second, "sampling interval")
	cmd.Flags().String("state", defaultStatePath, "state file path")
	return cmd
}

// runAgent builds the pipeline and runs the agent loop until ctx is cancelled.
// sink may be nil (journald, else JSON lines on errw).
func runAgent(ctx context.Context, g *globalOpts, interval time.Duration, statePath string, sink agent.Sink, errw io.Writer) error {
	if interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	p, err := loadPipeline(g)
	if err != nil {
		return err
	}
	lastCollect := time.Now()
	if sink == nil {
		sink = agent.DefaultSink()
		if _, ok := sink.(agent.JournalSink); !ok {
			sink = agent.NewJSONSink(errw)
		}
	}
	return agent.Run(ctx, agent.Config{
		Interval:  interval,
		StatePath: statePath,
		Host:      hostSummary(p),
		Sink:      sink,
		OnError:   func(err error) { logln(errw, "kerneltuner:", err) },
		Sample: func() (*metrics.Snapshot, error) {
			return metrics.Sample(p.Root)
		},
		Evaluate: func(values, rates map[string]float64) []advisor.Recommendation {
			if time.Since(lastCollect) >= factsRefresh {
				lastCollect = time.Now()
				if np, err := loadPipeline(g); err != nil {
					logln(errw, "kerneltuner: refresh facts:", err)
				} else {
					p = np
				}
			}
			return p.evaluate(values, rates)
		},
	})
}

func hostSummary(p *pipeline) string {
	h, _ := os.Hostname()
	if p.Facts != nil && p.Facts.Kernel.Release != "" {
		return h + " " + p.Facts.Kernel.Release
	}
	return h
}

func logln(w io.Writer, a ...any) { _, _ = fmt.Fprintln(w, a...) }
