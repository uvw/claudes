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
			header := []string{"ID", "TEAMMATES", "TASKS", "DIR", "LEAD"}
			if app.multi() {
				header = []string{"ID", "TEAMMATES", "TASKS", "ACCOUNT", "DIR", "LEAD"}
			}
			table := [][]string{header}
			for _, r := range rows {
				mates := "-"
				if len(r.Teammates) > 0 {
					mates = c.Magenta(shell.PrintableJoin(r.Teammates))
				}
				lead := render.NameCell(r.Label())
				if !r.Running {
					lead = c.Faint(lead + " (not running)")
				}
				row := []string{r.LeadID, mates, teams.TaskSummary(r.Tasks)}
				if app.multi() {
					row = append(row, shell.Printable(r.Account))
				}
				table = append(table, append(row, render.DirCell(r.Cwd), lead))
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include teams with no teammates and teams whose lead no longer runs")
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}

// teamRows reads every team of each account the command covers, and joins it
// with its lead from that account's claude agents.
func (app *App) teamRows(ctx context.Context, all bool) ([]teams.Row, error) {
	rows := []teams.Row{}
	for _, a := range app.selected() {
		list, err := a.Teams.List()
		if err != nil {
			return nil, err
		}
		sessions, err := app.listIn(ctx, []*account{a}, true)
		if err != nil {
			return nil, err
		}
		byID := map[string]claude.Session{}
		for _, s := range sessions {
			byID[s.SessionID] = s
		}
		for _, t := range list {
			s, known := byID[t.LeadSessionID]
			r := teams.RowOf(t, s, known)
			if !all && (!r.Running || len(r.Teammates) == 0) {
				continue
			}
			if r.Tasks, err = a.Teams.TaskList(t.Name); err != nil {
				return nil, err
			}
			if app.multi() {
				r.Account = a.Name
			}
			rows = append(rows, r)
		}
	}
	return rows, nil
}
