package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/version"
)

// row is a session as `claudes ls` shows it.
type row struct {
	claude.Session
	Pinned bool `json:"pinned"`
	// Version is the Claude Code version the session last ran, when known.
	Version  string `json:"version"`
	Outdated bool   `json:"outdated"`
	// Teammates are the agent team members the session leads that run now.
	Teammates []string `json:"teammates"`
}

func (app *App) lsCommand() *cobra.Command {
	var all, asJSON bool
	var out outputFlag
	cmd := &cobra.Command{
		Use:     "ls [TARGET...]",
		Aliases: []string{"list"},
		Short:   "List sessions, or the ones the targets match: pinned first, then busy, then newest",
		Long: `List sessions: pinned first, then busy, then newest. With targets, lists only
the sessions they match. A target with *, ?, or [ is a pattern over names and
ids, such as 'plan*'; in zsh, quote it or alias claudes to noglob claudes.`,
		ValidArgsFunction: app.completeSessions(anySession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			if asJSON {
				out.format = "json"
			}
			installed, _ := app.Agents.Version(cmd.Context())
			rows, err := app.rows(cmd.Context(), all, installed)
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
			app.printTable(app.Out, rows)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include finished background sessions")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the list as JSON, as -o json does")
	out.add(cmd, app.Config.Output.Format)
	cmd.MarkFlagsMutuallyExclusive("json", "output")
	return cmd
}

// rows lists sessions for ls and where. installed is the installed
// Claude Code version, or "" when unknown, which marks no session outdated.
func (app *App) rows(ctx context.Context, all bool, installed string) ([]row, error) {
	sessions, err := app.Agents.Sessions(ctx, all)
	if err != nil {
		return nil, err
	}
	pins, err := app.Jobs.PinSet()
	if err != nil {
		app.warn("claudes: cannot read pins: %v", err)
	}
	rows := make([]row, 0, len(sessions))
	for _, s := range sessions {
		r := row{Session: s, Pinned: s.JobID != "" && pins[s.JobID], Version: app.Jobs.Version(s.JobID), Teammates: app.Teams.Teammates(s.SessionID)}
		r.Outdated = installed != "" && r.Version != "" && version.Less(r.Version, installed)
		rows = append(rows, r)
	}
	slices.SortStableFunc(rows, func(a, b row) int {
		return cmp.Or(trueFirst(a.Pinned, b.Pinned), trueFirst(a.Busy(), b.Busy()), cmp.Compare(b.StartedAt, a.StartedAt))
	})
	return rows, nil
}

// trueFirst orders a before b when a is true and b is not, for sorting.
func trueFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

func (app *App) printTable(w io.Writer, rows []row) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No sessions.")
		return
	}
	c := app.colors
	table := [][]string{{"PINNED", "ID", "NAME", "KIND", "STATE", "STATUS", "TEAM", "AGE", "VERSION", "CWD"}}
	outdated := false
	for _, r := range rows {
		mark := "-"
		if r.Pinned {
			mark = c.Blue("yes")
		}
		ver := dash(r.Version)
		if r.Outdated {
			ver, outdated = c.Yellow(ver+" (old)"), true
		}
		team := "-"
		if n := len(r.Teammates); n > 0 {
			team = c.Magenta(fmt.Sprintf("lead+%d", n))
		}
		table = append(table, []string{mark, r.ID(), r.Label(), kind(r.Session), dash(r.State),
			app.status(dash(r.Status)), team, age(app.Now(), r.Started()), ver, c.Faint(homeShort(r.Cwd))})
	}
	style.Table(w, table)
	if outdated {
		fmt.Fprintln(w, "\n(old): behind the installed Claude Code, see claudes restart --outdated")
	}
}

// status colors a session's status: busy, waiting, and shell each in their own
// color, and the rest plain.
func (app *App) status(s string) string {
	switch s {
	case "busy":
		return app.colors.Green(s)
	case "waiting":
		return app.colors.Magenta(s)
	case "shell":
		return app.colors.Cyan(s)
	}
	return s
}

func kind(s claude.Session) string {
	if s.Interactive() {
		return "tty"
	}
	return "bg"
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// age is how long before now t was, such as 3d, or "-" when t is unknown.
func age(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func homeShort(path string) string {
	path = shell.Printable(path)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, ok := relWithin(home, path)
	switch {
	case !ok:
		return path
	case rel == ".":
		return "~"
	}
	return "~" + string(filepath.Separator) + rel
}

// relWithin is path relative to root, and whether path is root or below it.
func relWithin(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

func (app *App) whereCommand() *cobra.Command {
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "where TARGET...",
		Short: `Print the folders of the sessions the targets match, as in cd "$(claudes where NAME)"`,
		Long: `Print the folders of the sessions the targets match. With one match, prints the
bare folder, so cd "$(claudes where NAME)" works. With several, prints each
session's name and folder. With -o json or yaml, prints the same session
objects as claudes ls. A target with *, ?, or [ is a pattern over names and
ids, such as 'plan*'; in zsh, quote it or alias claudes to noglob claudes.`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: app.completeSessions(anySession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, _ := app.Agents.Version(cmd.Context())
			rows, err := app.rows(cmd.Context(), true, installed)
			if err != nil {
				return err
			}
			picked, err := target.Match(args, rows)
			if err != nil {
				return err
			}
			if out.format != "text" {
				return out.encode(app.Out, picked)
			}
			folders := make([][3]string, len(picked))
			for i, s := range picked {
				folders[i] = [3]string{s.Label(), s.ID(), s.Cwd}
			}
			return app.printFolders(folders)
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

// printFolders prints one folder bare, so cd "$(claudes where NAME)" works, or
// several as name, id, and folder, so sessions that share a name stay apart.
func (app *App) printFolders(folders [][3]string) error {
	if len(folders) == 1 {
		_, err := fmt.Fprintln(app.Out, shell.Printable(folders[0][2]))
		return err
	}
	table := make([][]string, 0, len(folders))
	for _, f := range folders {
		table = append(table, []string{shell.Printable(f[0]), f[1], app.colors.Faint(shell.Printable(f[2]))})
	}
	return style.Table(app.Out, table)
}

// listMatch keeps the rows any of targets match. A target that matches
// nothing is noted on stderr and leaves the rest as they are, so a listing
// shows what it found, as kubectl get does for a selector; where and the
// commands that act still fail on it.
func listMatch[T target.Ref](app *App, targets []string, rows []T) ([]T, error) {
	out, missed, err := target.Filter(targets, rows)
	for _, token := range missed {
		app.warn("claudes: no session matches %q", shell.Printable(token))
	}
	return out, err
}
