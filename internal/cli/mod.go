package cli

import "github.com/spf13/cobra"

func (app *App) modCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mod",
		Short: "Install the claudes mod, which reaches into running sessions",
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
