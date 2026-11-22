package history

import (
	"strconv"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/transcripts"
)

// ReviewToolCalls is the most tool calls a one-prompt session can make and
// still go to review. One that made more, such as a pull request review, did
// enough work to keep.
const ReviewToolCalls = 20

// TriageItem is one session or directory in the triage plan.
type TriageItem struct {
	Ref
	NameSource string    `json:"nameSource"`
	Cwd        string    `json:"cwd"`
	Size       int64     `json:"size"`
	LastActive time.Time `json:"lastActive"`
	// Why is the kind session gc removes it as, or why it is kept.
	Why        string             `json:"why"`
	LastPrompt string             `json:"lastPrompt"`
	Activity   *transcripts.Tally `json:"activity"`
}

// TriageReport is every past session sorted into remove, review, and keep.
type TriageReport struct {
	// Remove is what session gc --junk removes: what it removes by default,
	// and trivial and headless sessions.
	Remove []TriageItem `json:"remove"`
	// Review is the sessions only someone who reads their prompts can judge.
	Review []TriageItem `json:"review"`
	Keep   []TriageItem `json:"keep"`
}

// Triage sorts list into the report. items is what session gc would remove
// from list, keep what it must leave alone, and now when the triage runs.
func Triage(list []Session, items []Item, keep map[string]string, now time.Time) TriageReport {
	report := TriageReport{Remove: []TriageItem{}, Review: []TriageItem{}, Keep: []TriageItem{}}
	byID := map[string]Session{}
	for _, p := range list {
		byID[p.SessionID] = p
	}
	removed := map[string]bool{}
	for _, it := range items {
		t := TriageItem{Ref: Ref{ID: it.ID(), SessionID: it.SessionID, Name: it.Name}, Cwd: it.dir, Size: it.Size, Why: string(it.Why)}
		if p, ok := byID[it.SessionID]; ok && (it.Why == KindEmpty || it.Why == KindTrivial || it.Why == KindHeadless) {
			removed[it.SessionID] = true
			p.ReadTally(false)
			t = itemOf(p, string(it.Why))
			t.Size = it.Size
		}
		report.Remove = append(report.Remove, t)
	}

	recent := now.Add(-Floor)
	for _, p := range list {
		if removed[p.SessionID] {
			continue
		}
		if why, ok := keep[p.SessionID]; ok {
			report.Keep = append(report.Keep, itemOf(p, strings.TrimPrefix(why, "is ")))
			continue
		}
		if p.LastActive.After(recent) {
			report.Keep = append(report.Keep, itemOf(p, "changed in the last day"))
			continue
		}
		p.ReadTally(false)
		if why := keepWhy(p); why != "" {
			report.Keep = append(report.Keep, itemOf(p, why))
			continue
		}
		report.Review = append(report.Review, itemOf(p, "review"))
	}
	return report
}

// keepWhy is why a past session that session gc would not take is kept, or ""
// when someone has to read it to tell.
func keepWhy(p Session) string {
	a := p.Activity
	switch {
	case a == nil && p.tallyErr != nil:
		return "cannot read it"
	case a == nil:
		return "bigger than " + render.Size(transcripts.TallyLimit)
	case p.NameSource == transcripts.NameCustom:
		return "named with /rename"
	case a.Teammate:
		return "started by an agent team"
	case a.Edits > 0 || p.Disk.FileHistory > 0:
		return "changed files"
	case a.Prompts > 1:
		return "more than one prompt"
	case a.ToolCalls > ReviewToolCalls:
		return "more than " + strconv.Itoa(ReviewToolCalls) + " tool calls"
	case a.Prompts == 0 && a.Commands == 0:
		return "no prompt it could read"
	}
	return ""
}

func itemOf(p Session, why string) TriageItem {
	return TriageItem{Ref: p.Ref(), NameSource: p.NameSource, Cwd: p.Cwd,
		Size: p.Disk.Total, LastActive: p.LastActive, Why: why, LastPrompt: p.LastPrompt, Activity: p.Activity}
}

// TriageSize is the space items take.
func TriageSize(items []TriageItem) int64 {
	var n int64
	for _, t := range items {
		n += t.Size
	}
	return n
}

// gcCommandFor is the session gc command that removes exactly the kinds in
// items. Without trivial or headless sessions, that is session gc's default.
func gcCommandFor(items []TriageItem) string {
	kinds := map[Kind]bool{}
	for _, t := range items {
		kinds[Kind(t.Why)] = true
	}
	cmd := "claudes session gc"
	if !kinds[KindTrivial] && !kinds[KindHeadless] {
		return cmd
	}
	for _, k := range []struct {
		why  Kind
		flag string
	}{
		{KindEmpty, "--empty"}, {KindTrivial, "--trivial"}, {KindHeadless, "--headless"}, {KindCopy, "--copies"},
		{KindLeftover, "--leftovers"},
	} {
		if kinds[k.why] {
			cmd += " " + k.flag
		}
	}
	return cmd
}

// RemoveCommand is the session gc command that removes items. Unless narrowed,
// that is gcCommandFor's. Narrowed, it names each session, since session gc's
// kinds would take sessions -i or -l left out, and an older copy, which has no
// id, is left to session gc --copies.
func RemoveCommand(items []TriageItem, narrowed bool) string {
	if !narrowed {
		return gcCommandFor(items)
	}
	var ids []string
	copies := false
	for _, t := range items {
		if t.ID == "" {
			copies = true
		} else {
			ids = append(ids, t.ID)
		}
	}
	cmd := "claudes session gc " + strings.Join(ids, " ")
	if len(ids) == 0 {
		cmd = "claudes session gc --copies"
	}
	if copies && len(ids) > 0 {
		cmd += ", and claudes session gc --copies for the older copies, which have no id"
	}
	return cmd
}
