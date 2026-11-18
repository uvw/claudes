package cli

import (
	"errors"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) mcpUseCommand() *cobra.Command {
	var f switchFlags
	cmd := &cobra.Command{
		Use:   "use PROFILE TARGET",
		Short: "Turn on exactly a profile's MCP servers in a session, the rest off",
		Long: `Turn on the MCP servers PROFILE lists in config.yaml's mcp.profiles, and turn
off every other server mcp ls lists for TARGET's directory, as mcp on and mcp
off do: through the session's claudes mod, or with --restart. --in DIR changes
a directory where no session runs. A server the profile names that the
directory does not have gets a warning.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if f.in != "" {
				return cobra.ExactArgs(1)(cmd, args)
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return slices.Sorted(maps.Keys(app.Config.MCP.Profiles)), cobra.ShellCompDirectiveNoFileComp
			}
			return app.completeSessions(runningSession, false)(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]
			if profile == "" {
				return errors.New("name an MCP profile")
			}
			if err := mcp.CheckProfile(app.Config.MCP.Profiles, profile); err != nil {
				return err
			}
			_, tok, err := f.splitTarget(args[1:])
			if err != nil {
				return err
			}
			return app.switchMCP(cmd.Context(), "mcp use", &f, tok, func(servers []mcp.Server) (map[string]string, error) {
				want, missing := mcp.ProfileWant(servers, app.Config.MCP.Profiles[profile])
				for _, name := range missing {
					app.warn("claudes: profile %s names %q, which this directory does not have", profile, shell.Printable(name))
				}
				return want, nil
			})
		},
	}
	return app.switchCommand(cmd, &f)
}
