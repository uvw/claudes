package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/config"
)

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
	path := &cobra.Command{
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
	get := &cobra.Command{
		Use:               "get [KEY]",
		Short:             "Print one setting, or every setting with what it does",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeKeys,
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
				fmt.Fprintf(tw, "%s\t%s\t%s\n", k.Name, dash(value), k.About)
			}
			return tw.Flush()
		},
	}
	set := &cobra.Command{
		Use:               "set KEY VALUE",
		Short:             "Change one setting, keeping the rest of the file and its comments",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeKeys,
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
	edit := &cobra.Command{
		Use:   "edit",
		Short: "Open config.yaml in $VISUAL or $EDITOR, starting it if it is missing",
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
	cmd.AddCommand(path, get, set, edit)
	return cmd
}

// configPathErr is the error when claudes cannot even tell where config.yaml
// is, such as a relative $CLAUDES_CONFIG.
func (app *App) configPathErr() error {
	if app.ConfigPath == "" {
		return app.configErr
	}
	return nil
}

// loadConfig reads config.yaml again, so a broken file reports its error here.
func (app *App) loadConfig() (config.Config, error) {
	if err := app.configPathErr(); err != nil {
		return config.Config{}, err
	}
	return config.Load(app.ConfigPath)
}

func completeKeys(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]cobra.Completion, 0, len(config.Keys))
	for _, k := range config.Keys {
		out = append(out, cobra.CompletionWithDesc(k.Name, k.About))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
