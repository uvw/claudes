package cli

import (
	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/config"
)

func (app *App) configSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "set KEY VALUE",
		Short:             "Change one setting, keeping the rest of the file and its comments",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: app.completeKeys,
		RunE: func(_ *cobra.Command, args []string) error {
			if err := app.configPathErr(); err != nil {
				return err
			}
			if err := config.Set(app.ConfigPath, args[0], args[1]); err != nil {
				return err
			}
			app.say("%s = %s in %s", args[0], args[1], app.ConfigPath)
			return nil
		},
	}
}
