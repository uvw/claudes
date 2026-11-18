package cli

import (
	"bufio"
	"cmp"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

type planRow struct {
	id, name, label, cwd, note string
	// size is what removing it frees, or "".
	size string
}

// printPlan prints the sessions a command is about to act on, with their
// status.
func (app *App) printPlan(heading string, rows []planRow) {
	app.printPlanOf(heading, "session(s)", "STATUS", rows)
}

// printPlanOf is printPlan for rows that are not sessions, such as directories,
// or whose label is not a status. Like every table, it starts with the id and
// ends with the directory, the name, and any note; a column no row fills is left
// out.
func (app *App) printPlanOf(heading, noun, labelHeader string, rows []planRow) {
	app.say("%s %d %s:", heading, len(rows), noun)
	var labels, sizes, notes bool
	for _, r := range rows {
		app.record(func(rep *report) {
			rep.Plan = append(rep.Plan, reportEntry{ID: r.id, Name: r.name, Cwd: r.cwd, Result: r.label, Note: cmp.Or(r.note, r.size)})
		})
		labels, sizes, notes = labels || r.label != "", sizes || r.size != "", notes || r.note != ""
	}
	pick := func(r planRow, dirShown string) []string {
		cells := []string{dash(r.id)}
		if labels {
			cells = append(cells, r.label)
		}
		if sizes {
			cells = append(cells, r.size)
		}
		cells = append(cells, dirShown, r.name)
		if notes {
			cells = append(cells, r.note)
		}
		return cells
	}
	table := [][]string{pick(planRow{id: "ID", label: labelHeader, size: "SIZE", name: "NAME", note: "NOTE"}, "DIR")}
	for _, r := range rows {
		r.name = nameCell(r.name)
		table = append(table, pick(r, dirCell(r.cwd)))
	}
	var b strings.Builder
	_ = app.writeTable(&b, table, tableFit{margin: 2})
	for line := range strings.Lines(b.String()) {
		app.say("  %s", strings.TrimRight(line, " \n"))
	}
}

// gate ends a dry run once its plan is printed, and asks before going ahead
// with --confirm. proceed is false when the command should stop; err is
// errReported when the user said no.
func (app *App) gate(f planFlags, question string) (proceed bool, err error) {
	if f.dryRun {
		app.dryRunDone()
		return false, nil
	}
	if f.confirm && !app.confirmed("\n"+question) {
		app.say("Canceled.")
		return false, errReported
	}
	return true, nil
}

// dryRunDone ends a dry run's output.
func (app *App) dryRunDone() { app.say("\nDry run: nothing was changed.") }

// progress prints what happened to one session, in a column every command
// shares. note follows the name, such as " and pinged".
func (app *App) progress(verb string, s subject, note string) {
	paint := app.colors.Green
	if verb == "skipped" {
		paint = func(text string) string { return text }
	}
	app.say("  %s %s%s", style.Pad(verb, 10, paint), s.Label(), note)
	app.record(func(rep *report) {
		e := entryOf(s)
		e.Result, e.Note = verb, noteOf(note)
		rep.Results = append(rep.Results, e)
	})
}

// progressFailed prints a session that failed, to stderr.
func (app *App) progressFailed(s subject) {
	app.warn("  %s %s", style.Pad("FAILED", 10, app.errColors.Red), s.Label())
	app.record(func(rep *report) {
		e := entryOf(s)
		e.Result = "failed"
		rep.Results = append(rep.Results, e)
	})
}

type manualRef interface {
	subject
	ManualCommand() string
}

func printManual[T manualRef](app *App, heading string, refs []T) {
	printCommands(app, heading, "", refs)
}

// printCommands prints the command to run for each of refs. The report lists
// them under manual, or under leftOut with why when why is not "".
func printCommands[T manualRef](app *App, heading, why string, refs []T) {
	if len(refs) == 0 {
		return
	}
	app.say("")
	app.say("%s", heading)
	for _, r := range refs {
		app.say("  # %s", shell.Printable(r.Label()))
		app.say("  %s", r.ManualCommand())
		app.record(func(rep *report) {
			e := entryOf(r)
			e.Command = r.ManualCommand()
			if why != "" {
				e.Result = why
				rep.LeftOut = append(rep.LeftOut, e)
			} else {
				rep.Manual = append(rep.Manual, e)
			}
		})
	}
}

// printLeftOut names the sessions a bulk command did not act on, and why.
func (app *App) printLeftOut(sel selection, verb string) {
	why := "(--interactive stops them)"
	if sel.outdated {
		why = "(--outdated never picks them)"
	}
	printCommands(app, "Interactive sessions were left alone "+why+". "+
		"Each one resumes with this command, run in its terminal:", "interactive", sel.leftAlone)
	for _, group := range []struct {
		format, why string
		sessions    []claude.Session
	}{
		{"Pinned, not %s: %s. Add --pinned to include them. Named sessions replace the bulk selection, so name every session you want.", "pinned", sel.pinned},
		{"Leading an agent team, not %s: %s. Its teammates would stop with it and cannot be resumed. To include it anyway, name it with every other session you want.", "leads a team", sel.leads},
		{"Skipped, not %s: %s.", "skipped", sel.skipped},
		{"Not %s, because claude agents gave no job id: %s.", "no job id", sel.noJob},
	} {
		if len(group.sessions) > 0 {
			app.say("")
			app.say(group.format, verb, labels(group.sessions))
		}
		for _, s := range group.sessions {
			app.record(func(rep *report) {
				e := entryOf(s)
				e.Result = group.why
				rep.LeftOut = append(rep.LeftOut, e)
			})
		}
	}
}

func labelsOf[T interface{ Label() string }](items []T) []string {
	names := make([]string, 0, len(items))
	for _, s := range items {
		names = append(names, s.Label())
	}
	return names
}

func labels[T interface{ Label() string }](items []T) string {
	return strings.Join(labelsOf(items), ", ")
}

type failure struct {
	label, reason string
	// of is the session or directory that failed, when there is one, so the
	// report gives its id and directory.
	of subject
}

func (app *App) reportFailures(failures []failure) error {
	if len(failures) == 0 {
		return nil
	}
	app.warn("")
	app.errorLine("%d session(s) need attention:", len(failures))
	for _, f := range failures {
		app.errorLine("  %s: %s", f.label, f.reason)
		app.record(func(rep *report) {
			e := reportEntry{Name: f.label}
			if f.of != nil {
				e = entryOf(f.of)
			}
			e.Error = f.reason
			rep.Failures = append(rep.Failures, e)
		})
	}
	return errSomeFailed
}

func (app *App) confirmed(question string) bool {
	prompt := app.Out
	if app.rep != nil {
		prompt = app.Err
	}
	fmt.Fprintf(prompt, "%s [y/N] ", question)
	line, err := bufio.NewReader(app.In).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

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
	case gcItem:
		e.Cwd = s.dir
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
		Command: strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "), Plan: []reportEntry{}, Results: []reportEntry{}, LeftOut: []reportEntry{},
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
