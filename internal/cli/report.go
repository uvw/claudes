package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
)

// report is what a command that changes sessions did, printed as one JSON or
// YAML document with -o json or -o yaml. Every key is always present.
type report struct {
	Command string        `json:"command"`
	DryRun  bool          `json:"dryRun"`
	Plan    []reportEntry `json:"plan"`
	Results []reportEntry `json:"results"`
	// LeftOut are sessions the command chose not to act on, and why.
	LeftOut []reportEntry `json:"leftOut"`
	// Manual are commands the user has to run, such as resuming an
	// interactive session in its terminal.
	Manual []reportEntry `json:"manual"`
	// Failures are the sessions that need attention, with the reason.
	Failures []reportEntry `json:"failures"`
	// NotReached are sessions a Ctrl-C left as they were.
	NotReached []reportEntry `json:"notReached"`
	// Warnings are what the command printed to stderr as it went, such as
	// teammates that stop with their lead.
	Warnings []string `json:"warnings"`
	// Error is why the command stopped, or "".
	Error string `json:"error"`

	format  outputFlag
	printed bool
}

type reportEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	Result  string `json:"result"`
	Note    string `json:"note"`
	Command string `json:"command"`
	Error   string `json:"error"`
}

// subject is a session as a command reports it.
type subject interface {
	ID() string
	Label() string
}

func entryOf(s subject) reportEntry {
	e := reportEntry{ID: s.ID(), Name: s.Label()}
	switch s := s.(type) {
	case claude.Session:
		e.Cwd = s.Cwd
	case resumable:
		e.Cwd = s.Cwd
	case park.Entry:
		e.Cwd = s.Cwd
	case leftover:
		e.Cwd = s.Path
	}
	return e
}

// reports adds -o to a command that changes sessions.
func reports(cmd *cobra.Command) *cobra.Command {
	var o outputFlag
	o.add(cmd, "")
	return cmd
}

// startReport switches a command to a report when it was given -o json or
// -o yaml. Its text output is dropped from then on.
func (app *App) startReport(cmd *cobra.Command) {
	f := cmd.Flags().Lookup("output")
	if f == nil || cmd.Annotations[runsAlone] == "" || f.Value.String() == "text" {
		return
	}
	app.rep = &report{
		Command: cmd.Name(), Plan: []reportEntry{}, Results: []reportEntry{}, LeftOut: []reportEntry{},
		Manual: []reportEntry{}, Failures: []reportEntry{}, NotReached: []reportEntry{}, Warnings: []string{},
		format: outputFlag{format: f.Value.String()},
	}
	if d := cmd.Flags().Lookup("dry-run"); d != nil {
		app.rep.DryRun = d.Value.String() == "true"
	}
}

// record adds to the report, when there is one.
func (app *App) record(add func(r *report)) {
	if app.rep != nil {
		add(app.rep)
	}
}

// printReport prints the report once. It runs before claudes stops its own
// session, since nothing prints after that.
func (app *App) printReport() {
	if app.rep == nil || app.rep.printed {
		return
	}
	app.rep.printed = true
	if err := app.rep.format.encode(app.Out, app.rep); err != nil {
		app.warn("claudes: cannot print the report: %v", err)
	}
}

// noteOf turns a progress note such as ", which stopped meanwhile" into
// "which stopped meanwhile".
func noteOf(note string) string { return strings.TrimLeft(note, ", ") }
