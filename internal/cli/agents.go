package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/shell"
)

// agentsCommand is claudes agents, which opens agent view as claudes with no
// command does, and adds --profile. claude agents takes no argument named
// agents and no --profile, so neither hides one of its own.
func (app *App) agentsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents [--profile NAME] [ARGS...]",
		Short: "Open agent view, optionally with only an MCP profile's servers",
		Long: `Run claude agents with ARGS, as claudes with no command does. --profile NAME
gives agent view, and every session it starts, exactly the MCP servers that
NAME lists in config.yaml's mcp.profiles, and no others.

claudes writes the allowedMcpServers setting for the profile to a file only you
can read, in its state directory, and passes it with claude agents --settings.
Each server it can find is allowed by its URL or command: Claude Code matches a
plugin's server only by those, and once the list holds one command, a stdio
server only by its command. claudes never prints them.`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, rest, err := cutFlag(args, "profile")
			if err != nil {
				return errors.New("--profile needs a profile name")
			}
			if profile == "" {
				if slices.Contains(rest, "-h") || slices.Contains(rest, "--help") {
					return cmd.Help()
				}
				return app.runAgents(append([]string{"agents"}, rest...))
			}
			if hasFlag(rest, "settings") {
				return errors.New("--profile passes its own --settings: pass one or the other")
			}
			file, err := app.profileSettings(profile)
			if err != nil {
				return err
			}
			return app.runAgents(append([]string{"agents", "--settings", file}, rest...))
		},
	}
	// The command reads its own arguments, so the rest reach claude agents as
	// typed. The flag is declared for the help.
	cmd.Flags().String("profile", "", "give agent view only this profile's MCP servers")
	return cmd
}

// profileSettings writes the settings file that gives a session exactly
// profile's servers, and returns its path.
func (app *App) profileSettings(profile string) (string, error) {
	profiles := app.Config.MCP.Profiles
	if err := mcp.CheckProfile(profiles, profile); err != nil {
		return "", err
	}
	names := profiles[profile]
	dir, err := paths.Real("")
	if err != nil {
		return "", err
	}
	known := app.MCP.Known(dir)
	for _, n := range names {
		if !known[n] {
			app.warn("claudes: profile %s names %q, which matches no MCP server claudes can find", profile, shell.Printable(n))
		}
	}
	entries, missing := app.MCP.Allowed(dir, names)
	if len(missing) > 0 {
		shown := make([]string, len(missing))
		for i, m := range missing {
			shown[i] = shell.Printable(m)
		}
		return "", fmt.Errorf("profile %s names %s, and claudes cannot find the plugin's URL or command for it, "+
			"which Claude Code needs to allow it", profile, strings.Join(shown, ", "))
	}
	file, err := app.MCP.SaveAllowed(profile, entries)
	if err != nil {
		return "", err
	}
	app.log().Debug("wrote the profile's settings", "profile", profile, "servers", len(entries), "path", file)
	return file, nil
}

// runAgents runs claude with args in place of claudes, as passthrough does.
func (app *App) runAgents(args []string) error {
	if err := app.Exec("claude", args...); err != nil {
		return fmt.Errorf("cannot run claude %s: %w", args[0], err)
	}
	return nil
}

// cutFlag takes --name VALUE or --name=VALUE out of args, a claude command
// line, and returns VALUE, or "" when args do not hold the flag, and the rest
// as they were. Arguments from -- on are not flags.
func cutFlag(args []string, name string) (value string, rest []string, err error) {
	flag := "--" + name
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return value, append(rest, args[i:]...), nil
		case a == flag:
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s needs a value", flag)
			}
			value = args[i+1]
			i++
		case strings.HasPrefix(a, flag+"="):
			if value = strings.TrimPrefix(a, flag+"="); value == "" {
				return "", nil, fmt.Errorf("%s needs a value", flag)
			}
		default:
			rest = append(rest, a)
		}
	}
	return value, rest, nil
}

// hasFlag reports whether args, a claude command line, hold --name or
// --name=VALUE before any --.
func hasFlag(args []string, name string) bool {
	flag := "--" + name
	for _, a := range args {
		switch {
		case a == "--":
			return false
		case a == flag, strings.HasPrefix(a, flag+"="):
			return true
		}
	}
	return false
}
