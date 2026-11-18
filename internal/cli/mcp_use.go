package cli

import (
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) mcpUseCommand() *cobra.Command {
	var f mcpSetFlags
	cmd := &cobra.Command{
		Use:   "use PROFILE TARGET",
		Short: "Turn on exactly a profile's MCP servers in a running session, and the rest off",
		Long: `Turn on the MCP servers PROFILE lists in config.yaml's mcp.profiles, and turn off
every other server mcp ls lists for TARGET's directory, as mcp on and mcp off
do: through the session's claudes mod, or with --restart. --in DIR changes a
directory where no session runs. A server the profile names that the directory
does not have gets a warning.`,
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
			profiles := app.Config.MCP.Profiles
			keep, ok := profiles[args[0]]
			if !ok {
				return noProfile(args[0], slices.Sorted(maps.Keys(profiles)))
			}
			_, tok, err := f.splitTarget(args[1:])
			if err != nil {
				return err
			}
			return app.setMCP(cmd.Context(), "mcp use", &f, tok, func(servers []mcp.Server) (map[string]string, error) {
				want := map[string]string{}
				for _, s := range servers {
					want[s.Name] = mcp.Off
					if slices.Contains(keep, s.Name) {
						want[s.Name] = mcp.On
					}
				}
				for _, name := range keep {
					if _, ok := want[name]; !ok {
						app.warn("claudes: profile %s names %q, which this directory does not have", args[0], shell.Printable(name))
					}
				}
				return want, nil
			})
		},
	}
	f.add(app, cmd)
	return setCommand(cmd)
}
