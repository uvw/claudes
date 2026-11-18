package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
)

func (app *App) mcpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "List the MCP servers a session or directory has, and your MCP profiles",
		Long: `Commands for the MCP servers Claude Code gives a session. claudes reads them from
~/.claude.json, the settings files, .mcp.json files, and the enabled plugins,
and changes nothing. It never prints a server's command, arguments,
environment, headers, or URL, which can carry tokens.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.mcpLsCommand(), app.mcpProfilesCommand())
	return cmd
}

// mcpProfile is one profile, as mcp profiles prints it.
type mcpProfile struct {
	Name    string   `json:"name"`
	Servers []string `json:"servers"`
	// Unknown are the servers that match none claudes can find.
	Unknown []string `json:"unknown"`
}

func (app *App) mcpProfilesCommand() *cobra.Command {
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List the MCP profiles in config.yaml and the servers each keeps on",
		Long: `List the profiles in config.yaml's mcp.profiles and the servers each keeps on.
A server name that matches no server claudes can find gets a warning: one of
the current directory's, any directory's in ~/.claude.json, or an installed
plugin's. Change the profiles with claudes config edit.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			profiles := app.Config.MCP.Profiles
			dir, err := realDir("")
			if err != nil {
				return err
			}
			known := app.MCP.Known(dir)
			list := make([]mcpProfile, 0, len(profiles))
			for _, name := range slices.Sorted(maps.Keys(profiles)) {
				p := mcpProfile{Name: name, Servers: append([]string{}, profiles[name]...), Unknown: []string{}}
				for _, s := range p.Servers {
					if !known[s] {
						p.Unknown = append(p.Unknown, s)
						app.warn("claudes: profile %s names %q, which matches no MCP server claudes can find", name, shell.Printable(s))
					}
				}
				list = append(list, p)
			}
			if out.format != "text" {
				return out.encode(app.Out, list)
			}
			if len(list) == 0 {
				fmt.Fprintln(app.Out, "No MCP profiles. Add them under mcp.profiles with claudes config edit.")
				return nil
			}
			table := [][]string{{"NAME", "SERVERS"}}
			for _, p := range list {
				servers := make([]string, len(p.Servers))
				for i, s := range p.Servers {
					servers[i] = shell.Printable(s)
				}
				table = append(table, []string{p.Name, dash(strings.Join(servers, ", "))})
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}
