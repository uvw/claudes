package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
)

// midTurn is Session.MidTurn, plus a lead whose teammates are at work: its
// turn goes on through them, so a restart should ping it to carry on.
func (app *App) midTurn(s claude.Session) bool {
	return s.MidTurn() || len(app.Teams.Teammates(s.SessionID)) > 0
}

// teamNote names the teammates that would stop with s, or is "".
func (app *App) teamNote(s claude.Session) string {
	names := app.Teams.Teammates(s.SessionID)
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("leads %d teammate(s): %s, which stop with it and cannot be resumed", len(names), printableList(names))
}

// warnTeams warns about named sessions whose teammates would stop with them.
func (app *App) warnTeams(sessions []claude.Session) {
	for _, s := range sessions {
		if note := app.teamNote(s); note != "" && s.Running() {
			app.warn("claudes: %s %s.", s.Label(), note)
		}
	}
}

// printableList joins names read from Claude Code's files, with any control
// characters in them made visible.
func printableList(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = shell.Printable(n)
	}
	return strings.Join(out, ", ")
}

func joinNotes(notes ...string) string {
	var kept []string
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}

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
