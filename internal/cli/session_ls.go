package cli

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"time"

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
					dash(shell.Printable(p.Branch)), app.colors.Faint(homeShort(p.Cwd)), nameCell(p.Label())})
			}
			return style.Table(app.Out, table)
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

func (app *App) filterPast(list []pastSession, targets []string, in string, since time.Duration) ([]pastSession, error) {
	if len(targets) > 0 {
		var err error
		if list, err = listMatch(app, targets, list); err != nil {
			return nil, err
		}
	}
	var root string
	if in != "" {
		var err error
		if root, err = filepath.Abs(in); err != nil {
			return nil, err
		}
		// Sessions record the real directory, so a link in --in is followed.
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
	}
	out := list[:0]
	for _, p := range list {
		if root != "" && !within(p.Cwd, root) && !within(resolved(p.Cwd), root) {
			continue
		}
		if since > 0 && app.Now().Sub(p.LastActive) > since {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// resolved is dir with links followed, or dir when it is gone.
func resolved(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// within reports whether dir is root or a directory below it.
func within(dir, root string) bool {
	if dir == "" {
		return false
	}
	_, ok := relWithin(root, dir)
	return ok
}
