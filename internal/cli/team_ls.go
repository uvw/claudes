package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/teams"
)

func (app *App) teamLsCommand() *cobra.Command {
	var all bool
	var out render.Format
	cmd := &cobra.Command{
		Use:   "ls [LEAD...]",
		Short: "List agent teams at work: leads, teammates, and task progress",
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
			if out != render.Text {
				return out.Encode(app.Out, rows)
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
					mates = c.Magenta(shell.PrintableJoin(r.Teammates))
				}
				lead := render.NameCell(r.Label())
				if !r.Running {
					lead = c.Faint(lead + " (not running)")
				}
				table = append(table, []string{r.LeadID, mates, teams.TaskSummary(r.Tasks), render.DirCell(r.Cwd), lead})
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include teams with no teammates and teams whose lead no longer runs")
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}

// teamRows reads every team and joins it with its lead from claude agents.
func (app *App) teamRows(ctx context.Context, all bool) ([]teams.Row, error) {
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
	rows := []teams.Row{}
	for _, t := range list {
		s, known := byID[t.LeadSessionID]
		r := teams.RowOf(t, s, known)
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
