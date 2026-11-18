package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/render"
)

func (app *App) configGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "get [KEY]",
		Short:             "Print one setting, or every setting with what it does",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: app.completeKeys,
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := app.loadConfig()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				value, err := config.Get(cfg, args[0])
				if err != nil {
					return err
				}
				fmt.Fprintln(app.Out, value)
				return nil
			}
			tw := tabwriter.NewWriter(app.Out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tVALUE\tWHAT IT DOES")
			for _, k := range config.Keys {
				value, _ := config.Get(cfg, k.Name)
				fmt.Fprintf(tw, "%s\t%s\t%s\n", k.Name, render.Dash(value), k.About)
			}
			return tw.Flush()
		},
	}
}

// loadConfig reads config.yaml again, so a broken file reports its error here.
func (app *App) loadConfig() (config.Config, error) {
	if err := app.configPathErr(); err != nil {
		return config.Config{}, err
	}
	return config.Load(app.ConfigPath)
}
