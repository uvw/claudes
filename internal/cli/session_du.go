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

// duFolder is the space the sessions of one project folder take.
type duFolder struct {
	// Folder is where the newest of its sessions ran.
	Folder string `json:"folder"`
	// Project is the project folder under ~/.claude/projects.
	Project  string           `json:"project"`
	Sessions int              `json:"sessions"`
	Disk     transcripts.Disk `json:"disk"`
}

type duReport struct {
	Folders   []duFolder             `json:"folders"`
	Leftovers []transcripts.Leftover `json:"leftovers"`
	// Total includes the leftovers.
	Total int64 `json:"total"`
}

func (app *App) sessionDuCommand() *cobra.Command {
	var in string
	var since days
	var out outputFlag
	cmd := &cobra.Command{
		Use:     "du [TARGET...]",
		Aliases: []string{"df"},
		Short:   "Show how much disk space sessions take, by folder",
		Long: `Add up the space sessions take on disk, one line per project folder under
~/.claude/projects, biggest first: transcripts, subagent transcripts and tool
results (SIDECARS), older copies session mv left, and Claude Code's backups of
the files each session edited (HISTORY).

Leftovers are sidecar and file-history folders whose transcript is gone. They
count only when no target, --in, or --since narrows the sessions.`,
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
			if out.format != "text" {
				return out.encode(app.Out, report)
			}
			if len(report.Folders) == 0 && len(leftovers) == 0 {
				fmt.Fprintln(app.Out, "No sessions.")
				return nil
			}
			table := [][]string{{"TOTAL", "TRANSCRIPTS", "SIDECARS", "COPIES", "HISTORY", "SESSIONS", "FOLDER"}}
			var sum transcripts.Disk
			sessions := 0
			for _, f := range report.Folders {
				table = append(table, duRow(f.Disk, strconv.Itoa(f.Sessions), app.colors.Faint(homeShort(f.Folder))))
				sum.Add(f.Disk)
				sessions += f.Sessions
			}
			if len(leftovers) > 0 {
				var size int64
				for _, l := range leftovers {
					size += l.Size
				}
				table = append(table, []string{style.Size(size), "-", "-", "-", "-", "-",
					app.colors.Faint(fmt.Sprintf("%d leftover folder(s) with no transcript", len(leftovers)))})
			}
			table = append(table, duRow(sum, strconv.Itoa(sessions), "all"))
			table[len(table)-1][0] = style.Size(report.Total)
			return style.Table(app.Out, table)
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "only sessions run in this folder or below it")
	cmd.Flags().Var(&since, "since", "only sessions active within this long, such as 7d or 12h")
	cmd.Flags().Lookup("since").DefValue = ""
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

func duRow(d transcripts.Disk, sessions, folder string) []string {
	return []string{style.Size(d.Total), style.Size(d.Transcript), style.Size(d.Sidecar),
		style.Size(d.Copies), style.Size(d.FileHistory), sessions, folder}
}

// duTally groups list, which is newest first, by project folder, biggest
// first.
func duTally(list []pastSession, leftovers []transcripts.Leftover) duReport {
	report := duReport{Folders: []duFolder{}, Leftovers: leftovers}
	if report.Leftovers == nil {
		report.Leftovers = []transcripts.Leftover{}
	}
	index := map[string]int{}
	for _, p := range list {
		project := filepath.Base(filepath.Dir(p.Path))
		i, ok := index[project]
		if !ok {
			i = len(report.Folders)
			index[project] = i
			report.Folders = append(report.Folders, duFolder{Folder: p.Cwd, Project: project})
		}
		f := &report.Folders[i]
		f.Sessions++
		f.Disk.Add(p.Disk)
		report.Total += p.Disk.Total
	}
	for _, l := range leftovers {
		report.Total += l.Size
	}
	slices.SortStableFunc(report.Folders, func(a, b duFolder) int { return cmp.Compare(b.Disk.Total, a.Disk.Total) })
	return report
}
