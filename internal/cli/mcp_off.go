package cli

import (
	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
)

func (app *App) mcpOffCommand() *cobra.Command {
	var f switchFlags
	var all bool
	cmd := app.toggleCommand(mcp.Off, &f, &all)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return app.toggle(cmd.Context(), mcp.Off, &f, all, args)
	}
	return cmd
}
