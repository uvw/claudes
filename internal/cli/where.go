package cli

import (
	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/target"
)

func (app *App) whereCommand() *cobra.Command {
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "where TARGET...",
		Short: "Print the directories of the sessions the targets match",
		Long: `Print the directories of the sessions the targets match. With one match, prints
the bare directory, so cd "$(claudes where NAME)" works. With several, prints a
table of each session's id, directory, and name. With -o json or yaml, prints
the same session objects as claudes ls.

A target with *, ?, or [ is a pattern over names and ids, such as 'plan*'. In
zsh, quote it, or alias claudes to noglob claudes.`,
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
			found := make([]dirRow, len(picked))
			for i, s := range picked {
				found[i] = dirRow{id: s.ID(), dir: s.Cwd, name: s.Label()}
			}
			return app.printDirs(found)
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}
