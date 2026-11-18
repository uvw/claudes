package cli

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

func (app *App) sessionLsCommand() *cobra.Command {
	var in, sortBy string
	var since days
	var out outputFlag
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
			switch sortBy {
			case "active":
			case "size":
				slices.SortStableFunc(list, func(a, b pastSession) int { return cmp.Compare(b.Disk.Total, a.Disk.Total) })
			default:
				return fmt.Errorf("--sort takes active or size, not %q", sortBy)
			}
			if out.format != "text" {
				return out.encode(app.Out, list)
			}
			if len(list) == 0 {
				fmt.Fprintln(app.Out, "No sessions.")
				return nil
			}
			table := [][]string{{"ID", "RUNNING", "ACTIVE", "SIZE", "BRANCH", "DIR", "NAME"}}
			for _, p := range list {
				running := "-"
				if p.Running {
					running = app.colors.Green("yes")
				}
				table = append(table, []string{p.ShortID, running, age(app.Now(), p.LastActive), style.Size(p.Disk.Total),
					dash(shell.Printable(p.Branch)), dirCell(p.Cwd), nameCell(p.Label())})
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "only sessions run in this directory or below it")
	cmd.Flags().StringVar(&sortBy, "sort", "active", "order by active, newest first, or by size, biggest first")
	cmd.RegisterFlagCompletionFunc("sort", cobra.FixedCompletions([]cobra.Completion{"active", "size"}, cobra.ShellCompDirectiveNoFileComp))
	cmd.Flags().VarP(&since, "since", "l", "only sessions active within this long, such as 7d or 12h")
	cmd.Flags().Lookup("since").DefValue = ""
	out.add(cmd, app.Config.Output.Format)
	return cmd
}
