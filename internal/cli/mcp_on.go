package cli

import (
	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
)

func (app *App) mcpOnCommand() *cobra.Command {
	var f switchFlags
	var all bool
	cmd := app.toggleCommand(mcp.On, &f, &all)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return app.toggle(cmd.Context(), mcp.On, &f, all, args)
	}
	return cmd
}
