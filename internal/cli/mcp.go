package cli

import "github.com/spf13/cobra"

func (app *App) mcpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "List and switch a session's MCP servers, and list your profiles",
		Long: `Commands for the MCP servers Claude Code gives a session. claudes reads them
from ~/.claude.json, the settings files, .mcp.json files, and the enabled
plugins. on and off change only a directory's disabledMcpServers list in
~/.claude.json, through the session's claudes mod or by writing it. claudes
never prints a server's command, arguments, environment, headers, or URL,
which can carry tokens.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.mcpLsCommand(), app.mcpProfilesCommand(), app.mcpOnCommand(), app.mcpOffCommand(), app.mcpUseCommand())
	return cmd
}
