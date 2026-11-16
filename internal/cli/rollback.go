package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/apply"
)

func newRollbackCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback [ID]",
		Short: "Restore settings from an apply snapshot",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetBool("list")
			yes, _ := cmd.Flags().GetBool("yes")
			out := cmd.OutOrStdout()
			root := g.Root()
			if list {
				snaps, err := apply.List(root)
				if err != nil {
					return err
				}
				if g.JSON {
					return json.NewEncoder(out).Encode(snaps)
				}
				if len(snaps) == 0 {
					_, _ = fmt.Fprintln(out, "no snapshots")
				}
				for _, s := range snaps {
					_, _ = fmt.Fprintf(out, "%s\t%d live value(s)\t%d file(s)\t%s\n", s.ID, len(s.Live), len(s.Files), s.Version)
				}
				return nil
			}
			if err := requireRoot(g); err != nil {
				return err
			}
			var id string
			if len(args) == 1 {
				id = args[0]
			} else {
				if !yes {
					return errors.New("no snapshot ID given: pass an ID, or --yes to roll back the latest snapshot (see --list)")
				}
				snaps, err := apply.List(root)
				if err != nil {
					return err
				}
				if len(snaps) == 0 {
					return errors.New("no snapshots to roll back")
				}
				id = snaps[len(snaps)-1].ID
			}
			if err := apply.Rollback(root, id); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "rolled back snapshot %s\n", id)
			return nil
		},
	}
	cmd.Flags().Bool("list", false, "list available snapshots")
	cmd.Flags().Bool("yes", false, "roll back the latest snapshot when no ID is given")
	return cmd
}
