package cli

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) mcpLsCommand() *cobra.Command {
	var in, profile string
	var out render.Format
	cmd := &cobra.Command{
		Use:   "ls [TARGET]",
		Short: "List the MCP servers a session or directory has",
		Long: `List every MCP server Claude Code gives a session in a directory: TARGET's, past
or running, or --in DIR, or the current directory. STATE is on, off, or pending
for a project server in .mcp.json that nobody has approved yet. PROFILES names
the profiles in config.yaml's mcp.profiles that keep the server on.

--profile NAME adds what each server's state would be under that profile, which
keeps on only the servers it lists. It changes nothing.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: app.completePast(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && in != "" {
				return errors.New("name a session or pass --in, not both")
			}
			profiles := app.Config.MCP.Profiles
			if err := mcp.CheckProfile(profiles, profile); err != nil {
				return err
			}
			var dir string
			switch {
			case len(args) == 1:
				p, err := app.onePast(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				dir = p.Cwd
			default:
				var err error
				if dir, err = paths.Real(in); err != nil {
					return err
				}
			}
			// Each account keeps its own MCP settings, so each gets a listing.
			var lists []mcp.Listing
			for _, a := range app.selected() {
				servers, problems := a.MCP.Servers(dir)
				for _, err := range problems {
					app.warn("claudes: cannot read every MCP setting: %v", err)
				}
				list := mcp.NewListing(dir, profile, servers, profiles)
				if app.multi() {
					list.Account = app.accountLabel(a)
				}
				lists = append(lists, list)
			}
			if out != render.Text {
				if !app.multi() {
					return out.Encode(app.Out, lists[0])
				}
				return out.Encode(app.Out, lists)
			}
			for i, list := range lists {
				if i > 0 {
					fmt.Fprintln(app.Out)
				}
				if err := app.printMCP(list); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "list the servers of this directory, rather than the current one")
	cmd.Flags().StringVar(&profile, "profile", "", "also show each server's state under this profile from config.yaml")
	cmd.RegisterFlagCompletionFunc("profile", func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return slices.Sorted(maps.Keys(app.Config.MCP.Profiles)), cobra.ShellCompDirectiveNoFileComp
	})
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}

func (app *App) printMCP(list mcp.Listing) error {
	before, after := "MCP servers in ", ":"
	switch {
	case len(list.Servers) == 0:
		before, after = "No MCP servers in ", "."
	case list.Profile != "":
		after = ", and under profile " + list.Profile + ":"
	}
	if list.Account != "" {
		after = ", under account " + shell.Printable(list.Account) + after
	}
	fmt.Fprintln(app.Out, before+app.dirIn(list.Dir, render.Width(before+after))+after)
	if len(list.Servers) == 0 {
		return nil
	}
	header := []string{"NAME", "SCOPE", "STATE"}
	if list.Profile != "" {
		header = append(header, "PROFILE")
	}
	table := [][]string{append(header, "KIND", "PROFILES")}
	for _, s := range list.Servers {
		row := []string{shell.Printable(s.Name), s.Scope, app.mcpState(s.State)}
		if list.Profile != "" {
			row = append(row, app.mcpState(s.ProfileState))
		}
		table = append(table, append(row, render.Dash(s.Kind), render.Dash(strings.Join(s.Profiles, ", "))))
	}
	return app.writeTable(app.Out, table, tableFit{})
}

// mcpState colors a server's state. Pending is magenta, like a session that
// waits on the user.
func (app *App) mcpState(s string) string {
	switch s {
	case mcp.On:
		return app.colors.Green(s)
	case mcp.Pending:
		return app.colors.Magenta(s)
	}
	return app.colors.Faint(s)
}
