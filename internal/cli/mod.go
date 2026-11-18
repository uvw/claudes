package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/mod"
	"github.com/uvw/claudes/internal/style"
)

func (app *App) modCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mod",
		Short: "Install the claudes mod, which runs commands claudes asks for inside running sessions",
		Long: `The claudes mod is a Claude Code plugin that runs inside each session. It lets
claudes run a slash command in a running session, such as /mcp disable for
claudes mcp off, once the session is idle, without restarting it. It runs only
the commands it allows, and only those claudes asks for, through files in a
directory only you can read.

claude plugin commands install and remove it: claudes never writes Claude
Code's plugin files itself.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.modInstallCommand(), app.modUninstallCommand(), app.modStatusCommand())
	return cmd
}

func (app *App) modInstallCommand() *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the claudes mod for Claude Code",
		Long: `Add the claudes plugin marketplace to Claude Code and install claudes-mod from
it, for your user. --from takes the marketplace from a clone of the claudes
repository, another GitHub repository, or a URL, instead of ` + mod.DefaultSource + `.

Sessions you start from then on load the mod. In a session that already runs,
run /reload-plugins to load it there.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			source, err := modSource(from)
			if err != nil {
				return err
			}
			return app.installMod(cmd.Context(), source)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "a clone of the claudes repository, a GitHub OWNER/REPO, or a URL to install from")
	return cmd
}

// modSource is where --from points: a directory as an absolute path with its
// links followed, or anything else as given.
func modSource(from string) (string, error) {
	if from == "" {
		return mod.DefaultSource, nil
	}
	info, err := os.Stat(from)
	if err != nil || !info.IsDir() {
		return from, nil
	}
	dir, err := absDir(from)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "marketplace.json")); err != nil {
		return "", fmt.Errorf("%s has no .claude-plugin/marketplace.json: name a clone of the claudes repository", dir)
	}
	return dir, nil
}

func (app *App) installMod(ctx context.Context, source string) error {
	market, found, err := app.modMarketplace(ctx)
	if err != nil {
		return err
	}
	switch {
	case !found:
		if err := app.Agents.AddMarketplace(ctx, source); err != nil {
			return err
		}
	case !sameSource(market.Where(), source):
		return fmt.Errorf("the %s marketplace in Claude Code comes from %s. Run claudes mod uninstall first to install from %s",
			mod.Marketplace, market.Where(), source)
	}
	plugin, installed, err := app.modPlugin(ctx)
	if err != nil {
		return err
	}
	if installed {
		app.say("claudes-mod %s is already installed from %s.", plugin.Version, source)
		if !plugin.Enabled {
			app.say("It is turned off. Turn it on with: claude plugin enable %s", mod.ID)
		}
		return app.Mod.Prepare()
	}
	if err := app.Agents.InstallPlugin(ctx, mod.ID); err != nil {
		return err
	}
	if err := app.Mod.Prepare(); err != nil {
		return err
	}
	app.say("Installed claudes-mod from %s.", source)
	app.say("Sessions you start from now on load it. In a session that already runs, run /reload-plugins.")
	return nil
}

// sameSource compares two marketplace sources, following a directory's links.
func sameSource(a, b string) bool {
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

func (app *App) modUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the claudes mod and its marketplace from Claude Code",
		Long: `Uninstall claudes-mod, remove the claudes marketplace from Claude Code, and
delete the directory claudes and the mod share. A running session keeps the mod
until /reload-plugins or a restart, but finds no requests.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			_, installed, err := app.modPlugin(ctx)
			if err != nil {
				return err
			}
			if installed {
				if err := app.Agents.UninstallPlugin(ctx, mod.ID); err != nil {
					return err
				}
			}
			_, found, err := app.modMarketplace(ctx)
			if err != nil {
				return err
			}
			if found {
				if err := app.Agents.RemoveMarketplace(ctx, mod.Marketplace); err != nil {
					return err
				}
			}
			if err := app.Mod.Remove(); err != nil {
				return err
			}
			if !installed && !found {
				app.say("claudes-mod is not installed.")
				return nil
			}
			app.say("Uninstalled claudes-mod. Running sessions drop it at /reload-plugins or a restart.")
			return nil
		},
	}
}

func (app *App) modPlugin(ctx context.Context) (claude.Plugin, bool, error) {
	plugins, err := app.Agents.Plugins(ctx)
	if err != nil {
		return claude.Plugin{}, false, err
	}
	i := slices.IndexFunc(plugins, func(p claude.Plugin) bool { return p.ID == mod.ID })
	if i < 0 {
		return claude.Plugin{}, false, nil
	}
	return plugins[i], true, nil
}

func (app *App) modMarketplace(ctx context.Context) (claude.Marketplace, bool, error) {
	list, err := app.Agents.Marketplaces(ctx)
	if err != nil {
		return claude.Marketplace{}, false, err
	}
	i := slices.IndexFunc(list, func(m claude.Marketplace) bool { return m.Name == mod.Marketplace })
	if i < 0 {
		return claude.Marketplace{}, false, nil
	}
	return list[i], true, nil
}

// modState is what mod status prints with -o json or yaml.
type modState struct {
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Version   string `json:"version"`
	Scope     string `json:"scope"`
	// Source is where the claudes marketplace comes from, or "".
	Source string `json:"source"`
	// Dir is the directory claudes and the mod share.
	Dir      string       `json:"dir"`
	Sessions []modSession `json:"sessions"`
}

// modSession is a running session, and the mod version it loaded, or "".
type modSession struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	Version string `json:"version"`
}

func (app *App) modStatusCommand() *cobra.Command {
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Say whether the claudes mod is installed, and which running sessions loaded it",
		Long: `Say whether claudes-mod is installed and turned on, its version, and where it
comes from, then list the running sessions and the mod version each loaded. A
session loads the mod when it starts, or at /reload-plugins.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := app.modStatus(cmd.Context())
			if err != nil {
				return err
			}
			if out.format != "text" {
				return out.encode(app.Out, st)
			}
			return app.printModStatus(st)
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

func (app *App) modStatus(ctx context.Context) (modState, error) {
	st := modState{Dir: app.Mod.Dir, Sessions: []modSession{}}
	plugin, installed, err := app.modPlugin(ctx)
	if err != nil {
		return st, err
	}
	st.Installed, st.Enabled, st.Version, st.Scope = installed, plugin.Enabled, plugin.Version, plugin.Scope
	market, found, err := app.modMarketplace(ctx)
	if err != nil {
		return st, err
	}
	if found {
		st.Source = market.Where()
	}
	sessions, err := app.Agents.Sessions(ctx, false)
	if err != nil {
		return st, err
	}
	for _, s := range sessions {
		if s.Running() {
			st.Sessions = append(st.Sessions, modSession{ID: s.ID(), Name: s.Label(), Cwd: s.Cwd, Version: app.modLoaded(s)})
		}
	}
	return st, nil
}

// modLoaded is the mod version a running session loaded, or "" when it shows
// no sign of the mod since it started.
func (app *App) modLoaded(s claude.Session) string {
	p, ok, err := app.Mod.Presence(s.SessionID)
	if err != nil || !ok || p.Time().Before(s.Started()) {
		return ""
	}
	return p.Version
}

func (app *App) printModStatus(st modState) error {
	switch {
	case !st.Installed:
		app.say("claudes-mod is not installed. Install it with: claudes mod install")
	case !st.Enabled:
		app.say("claudes-mod %s is installed from %s, but turned off. Turn it on with: claude plugin enable %s", st.Version, dash(st.Source), mod.ID)
	default:
		app.say("claudes-mod %s is installed from %s, for scope %s.", st.Version, dash(st.Source), st.Scope)
	}
	app.say("Requests go through %s.", homeShort(st.Dir))
	if len(st.Sessions) == 0 {
		app.say("No running sessions.")
		return nil
	}
	app.say("")
	table := [][]string{{"ID", "MOD", "DIR", "NAME"}}
	for _, s := range st.Sessions {
		loaded := app.colors.Green(s.Version)
		if s.Version == "" {
			loaded = app.colors.Faint("not loaded")
		}
		table = append(table, []string{s.ID, loaded, app.colors.Faint(homeShort(s.Cwd)), nameCell(s.Name)})
	}
	return style.Table(app.Out, table)
}
