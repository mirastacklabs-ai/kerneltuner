package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/render"
)

func newGenerateCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Write sysctl.d, limits.d, systemd or tuned files (touches nothing)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, _ := cmd.Flags().GetString("format")
			outDir, _ := cmd.Flags().GetString("out")
			p, err := loadPipeline(g)
			if err != nil {
				return err
			}
			defer p.Close()
			values, rates, err := p.sample(cmdContext(cmd), 0)
			if err != nil {
				return err
			}
			recs := p.evaluate(values, rates)
			for _, c := range advisor.Conflicts(recs) {
				if !c.Resolved {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: rules disagree: %s\n", c)
				}
			}
			files, err := render.Generate(format, recs, p.meta())
			if err != nil {
				return err
			}
			if len(files) == 0 {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "nothing to generate: no matching recommendations")
				return nil
			}
			names := make([]string, 0, len(files))
			for n := range files {
				names = append(names, n)
			}
			sort.Strings(names)
			if outDir == "" {
				return printFiles(cmd, names, files)
			}
			for _, n := range names {
				dst := filepath.Join(outDir, filepath.FromSlash(n))
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return err
				}
				mode := os.FileMode(0o644)
				if n == render.FileNIC {
					mode = 0o755
				}
				if err := os.WriteFile(dst, []byte(files[n]), mode); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "wrote", dst)
			}
			return nil
		},
	}
	addFixtureFlag(cmd, g)
	cmd.Flags().String("format", "all", strings.Join(render.Formats, "|"))
	cmd.Flags().String("out", "", "write files into DIR instead of stdout")
	return cmd
}

// printFiles writes the bodies to stdout; with several files each is preceded
// by a banner naming its path.
func printFiles(cmd *cobra.Command, names []string, files map[string]string) error {
	w := cmd.OutOrStdout()
	for i, n := range names {
		if len(names) > 1 {
			if i > 0 {
				_, _ = fmt.Fprintln(w)
			}
			if _, err := fmt.Fprintf(w, "# ==== %s ====\n", n); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, files[n]); err != nil {
			return err
		}
	}
	return nil
}
