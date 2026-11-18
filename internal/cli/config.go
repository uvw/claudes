package cli

import "github.com/spf13/cobra"

func (app *App) configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or change claudes' settings in config.yaml",
		Long: `claudes reads its settings from config.yaml: $CLAUDES_CONFIG, or
$XDG_CONFIG_HOME/claudes/config.yaml, or ~/.config/claudes/config.yaml, or
%APPDATA%\claudes\config.yaml on Windows. Flags always win over it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.configPathCommand(), app.configGetCommand(), app.configSetCommand(), app.configEditCommand())
	return cmd
}
