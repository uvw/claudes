package cli

import (
	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mod"
)

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
