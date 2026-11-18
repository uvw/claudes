package render

import (
	"cmp"
	"strings"
)

// Report is what a command that changes sessions did, printed as one JSON or
// YAML document with -o json or -o yaml. Every key is always present, and
// scripts read them, so a key keeps its name.
type Report struct {
	Command string  `json:"command"`
	DryRun  bool    `json:"dryRun"`
	Plan    []Entry `json:"plan"`
	Results []Entry `json:"results"`
	// LeftOut are sessions the command chose not to act on, and why.
	LeftOut []Entry `json:"leftOut"`
	// Manual are commands the user has to run, such as resuming an
	// interactive session in its terminal.
	Manual []Entry `json:"manual"`
	// Failures are the sessions that need attention, with the reason.
	Failures []Entry `json:"failures"`
	// NotReached are sessions a Ctrl-C left as they were.
	NotReached []Entry `json:"notReached"`
	// Warnings are what the command printed to stderr as it went, such as
	// teammates that stop with their lead.
	Warnings []string `json:"warnings"`
	// Error is why the command stopped, or "".
	Error string `json:"error"`
}

// NewReport is an empty report of command, whose lists encode as [] rather
// than null.
func NewReport(command string, dryRun bool) *Report {
	return &Report{
		Command: command, DryRun: dryRun, Plan: []Entry{}, Results: []Entry{}, LeftOut: []Entry{},
		Manual: []Entry{}, Failures: []Entry{}, NotReached: []Entry{}, Warnings: []string{},
	}
}

// Entry is one session, or directory, in a report.
type Entry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	Result  string `json:"result"`
	Note    string `json:"note"`
	Command string `json:"command"`
	Error   string `json:"error"`
}

// Subject is a session, or a directory, as a command reports it.
type Subject interface {
	ID() string
	Label() string
	// Dir is the session's directory, or the directory itself.
	Dir() string
}

func EntryOf(s Subject) Entry { return Entry{ID: s.ID(), Name: s.Label(), Cwd: s.Dir()} }

// Result is the entry of s with what happened to it. note is how progress
// goes on after the name, such as ", which stopped meanwhile", and the entry
// keeps it without the leading comma.
func Result(s Subject, result, note string) Entry {
	e := EntryOf(s)
	e.Result, e.Note = result, strings.TrimLeft(note, ", ")
	return e
}

// Failure is a session, or something else, that needs attention.
type Failure struct {
	Label, Reason string
	// Of is the session or directory that failed, when there is one, so the
	// report gives its id and directory.
	Of Subject
}

// Entry is f as the report lists it.
func (f Failure) Entry() Entry {
	e := Entry{Name: f.Label}
	if f.Of != nil {
		e = EntryOf(f.Of)
	}
	e.Error = f.Reason
	return e
}

// PlanRow is a session, or a directory, a command is about to act on.
type PlanRow struct {
	ID, Name, Label, Cwd, Note string
	// Size is what removing it frees, or "".
	Size string
}

// Entry is r as the report's plan lists it.
func (r PlanRow) Entry() Entry {
	return Entry{ID: r.ID, Name: r.Name, Cwd: r.Cwd, Result: r.Label, Note: cmp.Or(r.Note, r.Size)}
}

// JoinNotes joins the notes of a plan row that are not empty with semicolons.
func JoinNotes(notes ...string) string {
	var kept []string
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}

// PlanTable is rows as a table, headed by labelHeader over their labels. Like
// every table, it starts with the id and ends with the directory, the name,
// and any note; a column no row fills is left out.
func PlanTable(labelHeader string, rows []PlanRow) [][]string {
	var labels, sizes, notes bool
	for _, r := range rows {
		labels, sizes, notes = labels || r.Label != "", sizes || r.Size != "", notes || r.Note != ""
	}
	pick := func(r PlanRow, dirShown string) []string {
		cells := []string{Dash(r.ID)}
		if labels {
			cells = append(cells, r.Label)
		}
		if sizes {
			cells = append(cells, r.Size)
		}
		cells = append(cells, dirShown, r.Name)
		if notes {
			cells = append(cells, r.Note)
		}
		return cells
	}
	table := [][]string{pick(PlanRow{ID: "ID", Label: labelHeader, Size: "SIZE", Name: NameHeader, Note: "NOTE"}, DirHeader)}
	for _, r := range rows {
		r.Name = NameCell(r.Name)
		table = append(table, pick(r, DirCell(r.Cwd)))
	}
	return table
}

// Labels joins the labels of items with commas.
func Labels[T interface{ Label() string }](items []T) string {
	names := make([]string, 0, len(items))
	for _, s := range items {
		names = append(names, s.Label())
	}
	return strings.Join(names, ", ")
}
