package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) mcpProfilesCommand() *cobra.Command {
	var out render.Format
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List the MCP profiles in config.yaml and the servers each keeps on",
		Long: `List the profiles in config.yaml's mcp.profiles and the servers each keeps on.
A server name that matches no server claudes can find gets a warning: one of
the current directory's, any directory's in ~/.claude.json, or an installed
plugin's. Change the profiles with claudes config edit.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			dir, err := paths.Real("")
			if err != nil {
				return err
			}
			list := mcp.Profiles(app.Config.MCP.Profiles, app.MCP.Known(dir))
			for _, p := range list {
				for _, s := range p.Unknown {
					app.warn("claudes: profile %s names %q, which matches no MCP server claudes can find", p.Name, shell.Printable(s))
				}
			}
			if out != render.Text {
				return out.Encode(app.Out, list)
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
				table = append(table, []string{p.Name, render.Dash(strings.Join(servers, ", "))})
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}
