package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/target"
)

func (app *App) sessionWhereCommand() *cobra.Command {
	var one bool
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "where TARGET...",
		Short: "Print the folders of past and running sessions",
		Long: `Print the folders of the sessions the targets match, past or running. With one
match, prints the bare folder. With several, prints each session's name and
folder, or fails with --one, which claudes cd uses. With --one, a target that
is exactly a name picks the newest session with that name.`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: app.completePast(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.pastSessions(cmd.Context())
			if err != nil {
				return err
			}
			var picked []pastSession
			if one && len(args) == 1 {
				p, err := pickPast(args[0], list)
				if err != nil {
					return err
				}
				picked = []pastSession{p}
			} else if picked, err = target.Match(args, list); err != nil {
				return err
			}
			if one && len(picked) > 1 {
				names := make([]string, len(picked))
				for i, p := range picked {
					names[i] = shell.Printable(p.Label()) + " (" + p.ShortID + ")"
				}
				return fmt.Errorf("%d sessions match: %s. Name one", len(picked), strings.Join(names, ", "))
			}
			if out.format != "text" {
				return out.encode(app.Out, picked)
			}
			folders := make([][3]string, len(picked))
			for i, p := range picked {
				folders[i] = [3]string{p.Label(), p.ShortID, p.Cwd}
			}
			return app.printFolders(folders)
		},
	}
	cmd.Flags().BoolVar(&one, "one", false, "fail unless exactly one session matches")
	out.add(cmd, app.Config.Output.Format)
	return cmd
}
