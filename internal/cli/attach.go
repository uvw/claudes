package cli

import "github.com/spf13/cobra"

func (app *App) attachCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "attach TARGET",
		Short:             "Open a background session in this terminal",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: app.completeSessions(backgroundSession, true),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.handOff(cmd.Context(), "attach", args[0], false)
		},
	}
}
