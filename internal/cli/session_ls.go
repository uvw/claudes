package cli

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/style"
)

func (app *App) sessionLsCommand() *cobra.Command {
	var in string
	sortBy := choiceFlag{value: "active", of: []string{"active", "size"}}
	since := app.daysFlag(0)
	var out outputFlag
	var wide, running, stopped bool
	cmd := &cobra.Command{
		Use:     "ls [TARGET...]",
		Aliases: []string{"list"},
		Short:   "List past and running sessions, newest activity first",
		Long: `List every session that has a transcript, newest activity first. Targets match
as in claudes ls: part of a name, the start of an id, or a pattern with *, ?,
or [.

SIZE is everything the session keeps on disk: its transcript, its subagent
transcripts and tool results, older copies session mv left, and Claude Code's
backups of the files it edited. claudes session du adds them up by directory.`,
		ValidArgsFunction: app.completePast(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.pastSessions(cmd.Context())
			if err != nil {
				return err
			}
			if list, err = app.filterPast(list, args, in, since.d); err != nil {
				return err
			}
			if running || stopped {
				list = slices.DeleteFunc(list, func(p pastSession) bool { return p.Running != running })
			}
			if sortBy.value == "size" {
				slices.SortStableFunc(list, func(a, b pastSession) int { return cmp.Compare(b.Disk.Total, a.Disk.Total) })
			}
			if out.format != "text" {
				return out.encode(app.Out, list)
			}
			if len(list) == 0 {
				fmt.Fprintln(app.Out, "No sessions.")
				return nil
			}
			header := []string{"ID", "RUNNING", "ACTIVE", "SIZE"}
			if wide {
				header = append(header, "BRANCH")
			}
			table := [][]string{append(header, "DIR", "NAME")}
			for _, p := range list {
				running := "-"
				if p.Running {
					running = app.colors.Green("yes")
				}
				cells := []string{p.ShortID, running, age(app.Now(), p.LastActive), style.Size(p.Disk.Total)}
				if wide {
					cells = append(cells, branchCell(p.Branch))
				}
				table = append(table, append(cells, dirCell(p.Cwd), nameCell(p.Label())))
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "only sessions run in this directory or below it")
	cmd.Flags().Var(&sortBy, "sort", "order by active, newest first, or by size, biggest first")
	cmd.RegisterFlagCompletionFunc("sort", cobra.FixedCompletions(sortBy.of, cobra.ShellCompDirectiveNoFileComp))
	cmd.Flags().VarP(&since, "since", "l", "only sessions active within this long, such as 7d, 12h, or today")
	cmd.Flags().Lookup("since").DefValue = ""
	cmd.Flags().BoolVar(&wide, "wide", false, "add the git branch each session ran in")
	cmd.Flags().BoolVar(&running, "running", false, "only sessions running now")
	cmd.Flags().BoolVar(&stopped, "stopped", false, "only sessions not running now")
	cmd.MarkFlagsMutuallyExclusive("running", "stopped")
	out.add(cmd, app.Config.Output.Format)
	return cmd
}
