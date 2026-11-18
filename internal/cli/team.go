package cli

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/teams"
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

// teamRow is a team as claudes team ls shows it.
type teamRow struct {
	Team      string       `json:"team"`
	LeadID    string       `json:"id"`
	SessionID string       `json:"sessionId"`
	Lead      string       `json:"lead"`
	Cwd       string       `json:"cwd"`
	Running   bool         `json:"running"`
	Teammates []string     `json:"teammates"`
	Tasks     []teams.Task `json:"tasks"`
}

func (r teamRow) Label() string { return cmp.Or(shell.Printable(r.Lead), r.LeadID) }

func (r teamRow) ID() string { return r.LeadID }

func (r teamRow) Aliases() []string { return claude.NonEmpty(r.Lead, r.LeadID, r.SessionID, r.Team) }

func (app *App) teamLsCommand() *cobra.Command {
	var all bool
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "ls [LEAD...]",
		Short: "List agent teams at work: their leads, teammates, and task progress",
		Long: `List the agent teams with teammates at work now: the lead, its teammates, and
how many of the team's tasks are done. With agent teams on, Claude Code keeps a
team for every session, so -a lists those too, and the ones whose lead no longer
runs, which claudes gc --teams removes.`,
		ValidArgsFunction: app.completeSessions(runningSession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := app.teamRows(cmd.Context(), all)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				if rows, err = listMatch(app, args, rows); err != nil {
					return err
				}
			}
			if out.format != "text" {
				return out.encode(app.Out, rows)
			}
			if len(rows) == 0 {
				fmt.Fprintln(app.Out, "No teams.")
				return nil
			}
			c := app.colors
			table := [][]string{{"ID", "TEAMMATES", "TASKS", "DIR", "LEAD"}}
			for _, r := range rows {
				mates := "-"
				if len(r.Teammates) > 0 {
					mates = c.Magenta(printableList(r.Teammates))
				}
				lead := nameCell(r.Label())
				if !r.Running {
					lead = c.Faint(lead + " (not running)")
				}
				table = append(table, []string{r.LeadID, mates, taskSummary(r.Tasks), dirCell(r.Cwd), lead})
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include teams with no teammates and teams whose lead no longer runs")
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

// teamRows reads every team and joins it with its lead from claude agents.
func (app *App) teamRows(ctx context.Context, all bool) ([]teamRow, error) {
	list, err := app.Teams.List()
	if err != nil {
		return nil, err
	}
	sessions, err := app.Agents.Sessions(ctx, true)
	if err != nil {
		return nil, err
	}
	byID := map[string]claude.Session{}
	for _, s := range sessions {
		byID[s.SessionID] = s
	}
	rows := []teamRow{}
	for _, t := range list {
		s, known := byID[t.LeadSessionID]
		r := teamRow{
			Team: t.Name, LeadID: cmp.Or(s.JobID, claude.ShortID(t.LeadSessionID)), SessionID: t.LeadSessionID,
			Lead: s.Name, Cwd: s.Cwd, Running: known && s.Running(), Teammates: t.Teammates(),
		}
		if r.Cwd == "" && len(t.Members) > 0 {
			r.Cwd = t.Members[0].Cwd
		}
		if !all && (!r.Running || len(r.Teammates) == 0) {
			continue
		}
		if r.Tasks, err = app.Teams.TaskList(t.Name); err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// taskSummary is how many of a team's tasks are done, and who works on the
// open ones.
func taskSummary(tasks []teams.Task) string {
	if len(tasks) == 0 {
		return "-"
	}
	done := 0
	var working []string
	for _, t := range tasks {
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			working = append(working, shell.Printable(cmp.Or(t.Owner, "#"+t.ID)))
		}
	}
	s := fmt.Sprintf("%d/%d done", done, len(tasks))
	if len(working) > 0 {
		s += ", working: " + strings.Join(working, ", ")
	}
	return s
}
