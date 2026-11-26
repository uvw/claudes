package cli

import "github.com/spf13/cobra"

func (app *App) sessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session",
		Aliases: []string{"s", "sessions"},
		Short:   "Find, resume, and move past sessions, from their transcripts",
		Long: `Commands for every Claude Code session on this computer, past and running, read
from the transcripts under ~/.claude/projects. claudes only reads them, except
for session mv, which copies one, session gc, which removes them, and session
restore, which puts back what they moved to the trash.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.sessionLsCommand(), app.sessionDuCommand(), app.sessionSearchCommand(), app.sessionShowCommand(), app.sessionTriageCommand(), app.sessionWhereCommand(),
		alone(app.sessionResumeCommand()), alone(app.sessionMvCommand()), alone(app.reports(app.sessionGcCommand())), alone(app.sessionRestoreCommand()))
	return app.withAccount(cmd)
}
