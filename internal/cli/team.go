package cli

import "github.com/spf13/cobra"

func (app *App) teamCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"t", "teams"},
		Short:   "See Claude Code agent teams: their leads, teammates, and tasks",
		Long: `Commands for Claude Code's agent teams, read from ~/.claude/teams and the shared
task lists under ~/.claude/tasks. claude agents lists only a team's lead; these
show its teammates and their tasks too.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.teamLsCommand())
	return cmd
}
