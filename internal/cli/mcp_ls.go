package cli

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

// mcpList is what mcp ls prints with -o json or yaml.
type mcpList struct {
	Dir string `json:"dir"`
	// Profile is --profile, or "".
	Profile string      `json:"profile"`
	Servers []mcpServer `json:"servers"`
}

type mcpServer struct {
	mcp.Server
	// Profiles are the config.yaml profiles that keep the server on.
	Profiles []string `json:"profiles"`
	// ProfileState is the server's state under --profile, or "".
	ProfileState string `json:"profileState"`
}

func (app *App) mcpLsCommand() *cobra.Command {
	var in, profile string
	var out outputFlag
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
			if _, ok := profiles[profile]; profile != "" && !ok {
				return noProfile(profile, slices.Sorted(maps.Keys(profiles)))
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
				if dir, err = realDir(in); err != nil {
					return err
				}
			}
			servers, problems := app.MCP.Servers(dir)
			for _, err := range problems {
				app.warn("claudes: cannot read every MCP setting: %v", err)
			}
			list := mcpList{Dir: dir, Profile: profile, Servers: make([]mcpServer, 0, len(servers))}
			for _, s := range servers {
				row := mcpServer{Server: s, Profiles: []string{}}
				for _, name := range slices.Sorted(maps.Keys(profiles)) {
					if slices.Contains(profiles[name], s.Name) {
						row.Profiles = append(row.Profiles, name)
					}
				}
				if profile != "" {
					row.ProfileState = mcp.Off
					if slices.Contains(profiles[profile], s.Name) {
						row.ProfileState = mcp.On
					}
				}
				list.Servers = append(list.Servers, row)
			}
			if out.format != "text" {
				return out.encode(app.Out, list)
			}
			return app.printMCP(list)
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "list the servers of this directory, rather than the current one")
	cmd.Flags().StringVar(&profile, "profile", "", "also show each server's state under this profile from config.yaml")
	cmd.RegisterFlagCompletionFunc("profile", func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return slices.Sorted(maps.Keys(app.Config.MCP.Profiles)), cobra.ShellCompDirectiveNoFileComp
	})
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

func (app *App) printMCP(list mcpList) error {
	before, after := "MCP servers in ", ":"
	switch {
	case len(list.Servers) == 0:
		before, after = "No MCP servers in ", "."
	case list.Profile != "":
		after = ", and under profile " + list.Profile + ":"
	}
	fmt.Fprintln(app.Out, before+app.dirIn(list.Dir, style.Width(before+after))+after)
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
		table = append(table, append(row, dash(s.Kind), dash(strings.Join(s.Profiles, ", "))))
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

func noProfile(name string, have []string) error {
	if len(have) == 0 {
		return fmt.Errorf("no MCP profile %q: config.yaml has no mcp.profiles yet", name)
	}
	return fmt.Errorf("no MCP profile %q. Profiles: %s", name, strings.Join(have, ", "))
}
