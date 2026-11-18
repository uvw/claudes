package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (app *App) configPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print where config.yaml is",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := app.configPathErr(); err != nil {
				return err
			}
			fmt.Fprintln(app.Out, app.ConfigPath)
			return nil
		},
	}
}
