package cli

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/transcripts"
)

// duDir is the space the sessions of one project directory take.
type duDir struct {
	// Dir is where the newest of its sessions ran.
	Dir string `json:"dir"`
	// Project is the project directory under ~/.claude/projects.
	Project  string           `json:"project"`
	Sessions int              `json:"sessions"`
	Disk     transcripts.Disk `json:"disk"`
}

// duSession is one of the sessions that take the most space.
type duSession struct {
	sessionRef
	Cwd     string           `json:"cwd"`
	Running bool             `json:"running"`
	Disk    transcripts.Disk `json:"disk"`
}

type duReport struct {
	Dirs []duDir `json:"dirs"`
	// Top is the biggest sessions, with --top.
	Top       []duSession            `json:"top"`
	Leftovers []transcripts.Leftover `json:"leftovers"`
	// Total includes the leftovers.
	Total int64 `json:"total"`
}

func (app *App) sessionDuCommand() *cobra.Command {
	var in string
	var since days
	var top countFlag
	var out outputFlag
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
			report := duTally(list, leftovers)
			report.Top = duTop(list, top.n)
			if out.format != "text" {
				return out.encode(app.Out, report)
			}
			if len(report.Dirs) == 0 && len(leftovers) == 0 {
				fmt.Fprintln(app.Out, "No sessions.")
				return nil
			}
			table := [][]string{{"TOTAL", "TRANSCRIPTS", "SIDECARS", "COPIES", "HISTORY", "SESSIONS", "DIR"}}
			var sum transcripts.Disk
			sessions := 0
			for _, f := range report.Dirs {
				table = append(table, duRow(f.Disk, strconv.Itoa(f.Sessions), dirCell(f.Dir)))
				sum.Add(f.Disk)
				sessions += f.Sessions
			}
			if len(leftovers) > 0 {
				var size int64
				for _, l := range leftovers {
					size += l.Size
				}
				table = append(table, []string{style.Size(size), "-", "-", "-", "-", "-",
					app.colors.Faint(fmt.Sprintf("%d leftover dir(s) with no transcript", len(leftovers)))})
			}
			table = append(table, duRow(sum, strconv.Itoa(sessions), "all"))
			table[len(table)-1][0] = style.Size(report.Total)
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
				table = append(table, []string{s.ID, style.Size(s.Disk.Total), style.Size(s.Disk.Sidecar), share,
					dirCell(s.Cwd), nameCell(s.Label())})
			}
			return app.writeTable(app.Out, table, tableFit{})
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "only sessions run in this directory or below it")
	cmd.Flags().VarP(&since, "since", "l", "only sessions active within this long, such as 7d or 12h")
	cmd.Flags().Lookup("since").DefValue = ""
	cmd.Flags().VarP(&top, "top", "t", "also list the N sessions that take the most space")
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

// duTop is the n sessions in list that take the most space, biggest first.
func duTop(list []pastSession, n int) []duSession {
	out := []duSession{}
	if n <= 0 {
		return out
	}
	sorted := slices.Clone(list)
	slices.SortStableFunc(sorted, func(a, b pastSession) int { return cmp.Compare(b.Disk.Total, a.Disk.Total) })
	for _, p := range sorted[:min(n, len(sorted))] {
		out = append(out, duSession{sessionRef: refOf(p), Cwd: p.Cwd, Running: p.Running, Disk: p.Disk})
	}
	return out
}

func duRow(d transcripts.Disk, sessions, dir string) []string {
	return []string{style.Size(d.Total), style.Size(d.Transcript), style.Size(d.Sidecar),
		style.Size(d.Copies), style.Size(d.FileHistory), sessions, dir}
}

// duTally groups list, which is newest first, by project directory, biggest
// first.
func duTally(list []pastSession, leftovers []transcripts.Leftover) duReport {
	report := duReport{Dirs: []duDir{}, Leftovers: leftovers}
	if report.Leftovers == nil {
		report.Leftovers = []transcripts.Leftover{}
	}
	index := map[string]int{}
	for _, p := range list {
		project := filepath.Base(filepath.Dir(p.Path))
		i, ok := index[project]
		if !ok {
			i = len(report.Dirs)
			index[project] = i
			report.Dirs = append(report.Dirs, duDir{Dir: p.Cwd, Project: project})
		}
		f := &report.Dirs[i]
		f.Sessions++
		f.Disk.Add(p.Disk)
		report.Total += p.Disk.Total
	}
	for _, l := range leftovers {
		report.Total += l.Size
	}
	slices.SortStableFunc(report.Dirs, func(a, b duDir) int { return cmp.Compare(b.Disk.Total, a.Disk.Total) })
	return report
}
