package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/history"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/transcripts"
)

func (app *App) sessionDuCommand() *cobra.Command {
	var in string
	since := app.daysFlag(0)
	var top countFlag
	var out render.Format
	cmd := &cobra.Command{
		Use:     "du [TARGET...]",
		Aliases: []string{"df"},
		Short:   "Show how much disk space sessions take, by directory",
		Long: `Add up the space sessions take on disk, one line per project directory under
~/.claude/projects, biggest first: transcripts, subagent transcripts and tool
results (SIDECARS), older copies session mv left, and Claude Code's backups of
the files each session edited (HISTORY).

Leftovers are sidecar and file-history directories whose transcript is gone.
They count only when no target, --in, or --since narrows the sessions.

--top N also lists the N sessions that take the most space, with the share their
sidecars take: tool results and subagent transcripts are often most of a big
session.`,
		ValidArgsFunction: app.completePast(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.pastSessions(cmd.Context())
			if err != nil {
				return err
			}
			all := len(args) == 0 && in == "" && since.d == 0
			var leftovers []transcripts.Leftover
			if all {
				sessions := make([]transcripts.Session, len(list))
				for i, p := range list {
					sessions[i] = p.Session
				}
				if leftovers, err = app.Transcripts.Leftovers(sessions); err != nil {
					return err
				}
			}
			if list, err = app.filterPast(list, args, in, since.d); err != nil {
				return err
			}
			report := history.UsageOf(list, leftovers)
			report.Top = history.Biggest(list, top.n)
			if out != render.Text {
				return out.Encode(app.Out, report)
			}
			if len(report.Dirs) == 0 && len(leftovers) == 0 {
				fmt.Fprintln(app.Out, "No sessions.")
				return nil
			}
			table := [][]string{{"TOTAL", "TRANSCRIPTS", "SIDECARS", "COPIES", "HISTORY", "SESSIONS", "DIR"}}
			var sum transcripts.Disk
			sessions := 0
			for _, f := range report.Dirs {
				table = append(table, history.UsageCells(f.Disk, strconv.Itoa(f.Sessions), render.DirCell(f.Dir)))
				sum.Add(f.Disk)
				sessions += f.Sessions
			}
			if len(leftovers) > 0 {
				var size int64
				for _, l := range leftovers {
					size += l.Size
				}
				table = append(table, []string{render.Size(size), "-", "-", "-", "-", "-",
					fmt.Sprintf("%d leftover dir(s) with no transcript", len(leftovers))})
			}
			table = append(table, history.UsageCells(sum, strconv.Itoa(sessions), "all"))
			table[len(table)-1][0] = render.Size(report.Total)
			if err := app.writeTable(app.Out, table, tableFit{}); err != nil || len(report.Top) == 0 {
				return err
			}
			fmt.Fprintf(app.Out, "\nThe %d biggest session(s):\n", len(report.Top))
			table = [][]string{{"ID", "TOTAL", "SIDECARS", "SHARE", "DIR", "NAME"}}
			for _, s := range report.Top {
				share := "-"
				if s.Disk.Total > 0 {
					share = strconv.FormatInt(s.Disk.Sidecar*100/s.Disk.Total, 10) + "%"
				}
				table = append(table, []string{s.ID, render.Size(s.Disk.Total), render.Size(s.Disk.Sidecar), share,
					render.DirCell(s.Cwd), render.NameCell(s.Label())})
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "only sessions run in this directory or below it")
	cmd.Flags().VarP(&since, "since", "l", "only sessions active within this long, such as 7d, 12h, or today")
	cmd.Flags().Lookup("since").DefValue = ""
	cmd.Flags().VarP(&top, "top", "t", "also list the N sessions that take the most space")
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}
