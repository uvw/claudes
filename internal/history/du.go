package history

import (
	"cmp"
	"path/filepath"
	"slices"

	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/transcripts"
)

// DirUsage is the space the sessions of one project directory take.
type DirUsage struct {
	// Dir is where the newest of its sessions ran.
	Dir string `json:"dir"`
	// Project is the project directory under ~/.claude/projects.
	Project  string           `json:"project"`
	Sessions int              `json:"sessions"`
	Disk     transcripts.Disk `json:"disk"`
}

// SessionUsage is one of the sessions that take the most space.
type SessionUsage struct {
	Ref
	Cwd     string           `json:"cwd"`
	Running bool             `json:"running"`
	Disk    transcripts.Disk `json:"disk"`
}

// Usage is what session du reports.
type Usage struct {
	Dirs []DirUsage `json:"dirs"`
	// Top is the biggest sessions, with --top.
	Top       []SessionUsage         `json:"top"`
	Leftovers []transcripts.Leftover `json:"leftovers"`
	// Total includes the leftovers.
	Total int64 `json:"total"`
}

// UsageOf groups list, which is newest first, by project directory, biggest
// first, and adds the leftovers to the total.
func UsageOf(list []Session, leftovers []transcripts.Leftover) Usage {
	report := Usage{Dirs: []DirUsage{}, Leftovers: leftovers}
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
			report.Dirs = append(report.Dirs, DirUsage{Dir: p.Cwd, Project: project})
		}
		f := &report.Dirs[i]
		f.Sessions++
		f.Disk.Add(p.Disk)
		report.Total += p.Disk.Total
	}
	for _, l := range leftovers {
		report.Total += l.Size
	}
	slices.SortStableFunc(report.Dirs, func(a, b DirUsage) int { return cmp.Compare(b.Disk.Total, a.Disk.Total) })
	return report
}

// Biggest is the n sessions in list that take the most space, biggest first.
func Biggest(list []Session, n int) []SessionUsage {
	out := []SessionUsage{}
	if n <= 0 {
		return out
	}
	sorted := slices.Clone(list)
	slices.SortStableFunc(sorted, func(a, b Session) int { return cmp.Compare(b.Disk.Total, a.Disk.Total) })
	for _, p := range sorted[:min(n, len(sorted))] {
		out = append(out, SessionUsage{Ref: p.Ref(), Cwd: p.Cwd, Running: p.Running, Disk: p.Disk})
	}
	return out
}

// UsageCells is a row of session du's table: d's sizes, then sessions and dir.
func UsageCells(d transcripts.Disk, sessions, dir string) []string {
	return []string{render.Size(d.Total), render.Size(d.Transcript), render.Size(d.Sidecar),
		render.Size(d.Copies), render.Size(d.FileHistory), sessions, dir}
}
