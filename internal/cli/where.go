package cli

import (
	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/target"
)

func (app *App) whereCommand() *cobra.Command {
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "where TARGET...",
		Short: `Print the directories of the sessions the targets match, as in cd "$(claudes where NAME)"`,
		Long: `Print the directories of the sessions the targets match. With one match, prints the
bare directory, so cd "$(claudes where NAME)" works. With several, prints each
session's name, id, and directory, which it shortens to fit as every table
does. With -o json or yaml, prints the same session objects as claudes ls. A target with *, ?, or [ is a pattern over names and
ids, such as 'plan*'; in zsh, quote it or alias claudes to noglob claudes.`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: app.completeSessions(anySession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, _ := app.Agents.Version(cmd.Context())
			rows, err := app.rows(cmd.Context(), true, installed)
			if err != nil {
				return err
			}
			picked, err := target.Match(args, rows)
			if err != nil {
				return err
			}
			if out.format != "text" {
				return out.encode(app.Out, picked)
			}
			found := make([][3]string, len(picked))
			for i, s := range picked {
				found[i] = [3]string{s.Label(), s.ID(), s.Cwd}
			}
			return app.printDirs(found)
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}
