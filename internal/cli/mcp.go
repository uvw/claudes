package cli

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

func (app *App) mcpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "List, turn on, and turn off the MCP servers a session has, and list your MCP profiles",
		Long: `Commands for the MCP servers Claude Code gives a session. claudes reads them from
~/.claude.json, the settings files, .mcp.json files, and the enabled plugins.
on and off change one thing: a directory's disabledMcpServers list in
~/.claude.json, through the session's claudes mod or by writing it. claudes
never prints a server's command, arguments, environment, headers, or URL,
which can carry tokens.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.mcpLsCommand(), app.mcpProfilesCommand(), app.mcpSetCommand("on"), app.mcpSetCommand("off"))
	return cmd
}

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
		Short: "List the MCP servers a session's directory, or another directory, has",
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
				if dir, err = absDir(in); err != nil {
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
	where := homeShort(list.Dir)
	if len(list.Servers) == 0 {
		fmt.Fprintf(app.Out, "No MCP servers in %s.\n", where)
		return nil
	}
	if list.Profile == "" {
		fmt.Fprintf(app.Out, "MCP servers in %s:\n", where)
	} else {
		fmt.Fprintf(app.Out, "MCP servers in %s, and under profile %s:\n", where, list.Profile)
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
	return style.Table(app.Out, table)
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
			dir, err := absDir("")
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
			return style.Table(app.Out, table)
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

func noProfile(name string, have []string) error {
	if len(have) == 0 {
		return fmt.Errorf("no MCP profile %q: config.yaml has no mcp.profiles yet", name)
	}
	return fmt.Errorf("no MCP profile %q. Profiles: %s", name, strings.Join(have, ", "))
}

// absDir is dir as an absolute path with its links followed, as Claude Code
// records a directory, or the current directory when dir is "".
func absDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return abs, nil
}
