package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
)

// agentsHeader heads the column that shows a session's agent team and the
// subagents it runs.
const agentsHeader = "AGENTS"

// maxDoing is the most characters of DOING a table shows when it does not go
// to a terminal.
const maxDoing = 50

func (app *App) lsCommand() *cobra.Command {
	var all, asJSON, wide bool
	var out outputFlag
	cmd := &cobra.Command{
		Use:     "ls [TARGET...]",
		Aliases: []string{"list"},
		Short:   "List sessions: pinned first, then busy, then newest",
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
			return app.printTable(app.Out, rows, wide)
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include finished background sessions")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the list as JSON, as -o json does")
	cmd.Flags().BoolVar(&wide, "wide", false, "add the background shells, tokens, pull requests, and what each session is doing")
	out.add(cmd, app.Config.Output.Format)
	cmd.MarkFlagsMutuallyExclusive("json", "output")
	return cmd
}

func (app *App) printTable(w io.Writer, rows []row, wide bool) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No sessions.")
		return err
	}
	c := app.colors
	header := []string{"ID", "PINNED", "KIND", "STATE", "STATUS", agentsHeader}
	if wide {
		header = append(header, "SHELLS", "TOKENS", "PRS")
	}
	header = append(header, "AGE", "VERSION", "DIR", "NAME")
	if wide {
		header = append(header, "DOING")
	}
	table := [][]string{header}
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
		cells := []string{r.ID(), mark, kind(r.Session), dash(r.State), app.status(dash(r.Status)), app.agents(r)}
		if wide {
			cells = append(cells, countCell(r.Shells), tokenCount(r.Tokens), countCell(r.PRs))
		}
		cells = append(cells, age(app.Now(), r.Started()), ver, dirCell(r.Cwd), nameCell(r.Label()))
		if wide {
			cells = append(cells, doing(r))
		}
		table = append(table, cells)
	}
	if err := app.writeTable(w, table, tableFit{fitLast: wide, lastCap: maxDoing}); err != nil || !outdated {
		return err
	}
	_, err := fmt.Fprintln(w, "\n(old): behind the installed Claude Code, see claudes restart --outdated")
	return err
}

// agents is the AGENTS cell: lead+N for the agent team a session leads, and
// how many subagents it runs now.
func (app *App) agents(r row) string {
	var parts []string
	if n := len(r.Teammates); n > 0 {
		parts = append(parts, app.colors.Magenta(fmt.Sprintf("lead+%d", n)))
	}
	if r.Subagents != nil && *r.Subagents > 0 {
		parts = append(parts, strconv.Itoa(*r.Subagents))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

// doing is the DOING cell, which writeTable cuts to fit.
func doing(r row) string {
	if r.Doing == nil {
		return "-"
	}
	return shell.Printable(*r.Doing)
}

func countCell(n *int) string {
	if n == nil || *n == 0 {
		return "-"
	}
	return strconv.Itoa(*n)
}

// tokenCount is a token count as agent view prints it, such as 1.4M.
func tokenCount(n *int64) string {
	if n == nil {
		return "-"
	}
	if *n < 1000 {
		return strconv.FormatInt(*n, 10)
	}
	v, unit := float64(*n), ""
	for _, unit = range []string{"K", "M", "B"} {
		// Moves on at 999.5, which would round up to 1000.
		if v /= 1000; v < 999.5 {
			break
		}
	}
	if v < 9.95 {
		return fmt.Sprintf("%.1f%s", v, unit)
	}
	return fmt.Sprintf("%.0f%s", v, unit)
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
