package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mod"
	"github.com/uvw/claudes/internal/paths"
)

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
	case !paths.Same(market.Where(), source):
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

// modSource is where claudes mod install --from points: a directory as a real
// absolute path, which must hold the claudes marketplace, or anything else,
// such as a GitHub OWNER/REPO, as given. An empty from is mod.DefaultSource.
func modSource(from string) (string, error) {
	if from == "" {
		return mod.DefaultSource, nil
	}
	if info, err := os.Stat(from); err != nil || !info.IsDir() {
		return from, nil //nolint:nilerr // a source that is no directory is a repository or a URL
	}
	dir, err := paths.Real(from)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "marketplace.json")); err != nil {
		return "", fmt.Errorf("%s has no .claude-plugin/marketplace.json: name a clone of the claudes repository", dir)
	}
	return dir, nil
}
