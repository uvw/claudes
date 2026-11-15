package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

func (app *App) sessionLsCommand() *cobra.Command {
	var in string
	var since days
	var out outputFlag
	cmd := &cobra.Command{
		Use:     "ls [TARGET...]",
		Aliases: []string{"list"},
		Short:   "List past and running sessions, newest activity first",
		Long: `List every session that has a transcript, newest activity first. Targets match
as in claudes ls: part of a name, the start of an id, or a pattern with *, ?,
or [.`,
		ValidArgsFunction: app.completePast(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.pastSessions(cmd.Context())
			if err != nil {
				return err
			}
			if list, err = app.filterPast(list, args, in, since.d); err != nil {
				return err
			}
			if out.format != "text" {
				return out.encode(app.Out, list)
			}
			if len(list) == 0 {
				fmt.Fprintln(app.Out, "No sessions.")
				return nil
			}
			table := [][]string{{"NAME", "ID", "STATE", "ACTIVE", "BRANCH", "CWD"}}
			for _, p := range list {
				state := "-"
				if p.Running {
					state = app.colors.Green("running")
				}
				table = append(table, []string{shell.Printable(p.Label()), p.ShortID, state,
					age(app.Now(), p.LastActive), dash(shell.Printable(p.Branch)), app.colors.Faint(homeShort(p.Cwd))})
			}
			return style.Table(app.Out, table)
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "only sessions run in this folder or below it")
	cmd.Flags().Var(&since, "since", "only sessions active within this long, such as 7d or 12h")
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
		// Sessions record the real folder, so a link in --in is followed.
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
	}
	out := list[:0]
	for _, p := range list {
		if root != "" && !within(p.Cwd, root) {
			continue
		}
		if since > 0 && app.Now().Sub(p.LastActive) > since {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// within reports whether dir is root or a folder below it.
func within(dir, root string) bool {
	if dir == "" {
		return false
	}
	_, ok := relWithin(root, dir)
	return ok
}
