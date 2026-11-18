package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/render"
)

// agentsHeader heads the column that shows a session's agent team and the
// subagents it runs.
const agentsHeader = "AGENTS"

// maxDoing is the most characters of DOING a table shows when it does not go
// to a terminal.
const maxDoing = 50

func (app *App) lsCommand() *cobra.Command {
	var all, asJSON, wide bool
	var out render.Format
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
				out = render.JSON
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
			if out != render.Text {
				return out.Encode(app.Out, rows)
			}
			return app.printTable(app.Out, rows, wide)
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include finished background sessions")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the list as JSON, as -o json does")
	cmd.Flags().BoolVar(&wide, "wide", false, "add the background shells, tokens, pull requests, and what each session is doing")
	app.addOutput(cmd, &out, app.Config.Output.Format)
	cmd.MarkFlagsMutuallyExclusive("json", "output")
	return cmd
}

func (app *App) printTable(w io.Writer, rows []sessionRow, wide bool) error {
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
		ver := render.Dash(r.Version)
		if r.Outdated {
			ver, outdated = c.Yellow(ver+" (old)"), true
		}
		cells := []string{r.ID(), mark, r.Mode(), render.Dash(r.State), app.status(render.Dash(r.Status)), app.agents(r)}
		if wide {
			cells = append(cells, render.CountCell(r.Shells), render.Tokens(r.Tokens), render.CountCell(r.PRs))
		}
		cells = append(cells, render.Age(app.Now(), r.Started()), ver, render.DirCell(r.Cwd), render.NameCell(r.Label()))
		if wide {
			cells = append(cells, doingCell(r))
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
func (app *App) agents(r sessionRow) string {
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
