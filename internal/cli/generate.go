package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
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
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			for _, n := range names {
				mode := os.FileMode(0o644)
				if n == render.FileNIC {
					mode = 0o755
				}
				if err := writeNoFollow(outDir, filepath.FromSlash(n), []byte(files[n]), mode); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "wrote", facts.SafeText(filepath.Join(outDir, filepath.FromSlash(n))))
			}
			return nil
		},
	}
	addFixtureFlag(cmd, g)
	cmd.Flags().String("format", "all", strings.Join(render.Formats, "|"))
	cmd.Flags().String("out", "", "write files into DIR instead of stdout")
	return cmd
}

// writeNoFollow writes rel below dir without following symlinks: not at the
// final component and not in any directory on the way. Missing directories
// are created. Paths cannot escape dir.
func writeNoFollow(dir, rel string, data []byte, mode os.FileMode) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	rel = filepath.Clean(rel)
	parts := strings.Split(rel, string(filepath.Separator))
	cur := ""
	for i, p := range parts {
		cur = filepath.Join(cur, p)
		st, err := root.Lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if i < len(parts)-1 {
				if err := root.Mkdir(cur, 0o755); err != nil {
					return err
				}
			}
		case err != nil:
			return err
		case st.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("refusing to write through symlink %s", filepath.Join(dir, cur))
		}
	}
	f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
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
