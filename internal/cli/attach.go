package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/target"
)

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

// handOff runs claude NAME on the background session token names, such as
// claude logs or claude attach. finished includes finished sessions.
func (app *App) handOff(ctx context.Context, name, token string, finished bool) error {
	sessions, err := app.Agents.Sessions(ctx, finished)
	if err != nil {
		return err
	}
	picked, err := target.Resolve([]string{token}, sessions)
	if err != nil {
		return err
	}
	s := picked[0]
	if s.JobID == "" {
		return refused(fmt.Errorf("%s is an interactive session: claude %s works on background sessions. claudes logs -f follows any session", s.Label(), name))
	}
	return app.Exec("claude", name, s.JobID)
}
