package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/config"
)

func (app *App) configEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open config.yaml in $VISUAL or $EDITOR, creating it if missing",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := app.configPathErr(); err != nil {
				return err
			}
			if err := config.Create(app.ConfigPath); err != nil {
				return err
			}
			if err := app.Edit(app.ConfigPath); err != nil {
				return err
			}
			if _, err := config.Load(app.ConfigPath); err != nil {
				return fmt.Errorf("the file was saved, but claudes cannot use it: %w", err)
			}
			return nil
		},
	}
}
