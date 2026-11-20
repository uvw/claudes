package cli

import "github.com/spf13/cobra"

func (app *App) mcpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "List the MCP servers a session has, and your MCP profiles",
		Long: `Commands for the MCP servers Claude Code gives a session. claudes reads them
from ~/.claude.json, the settings files, .mcp.json files, and the enabled
plugins, and changes nothing. It never prints a server's command, arguments,
environment, headers, or URL, which can carry tokens.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.mcpLsCommand(), app.mcpProfilesCommand())
	return cmd
}
