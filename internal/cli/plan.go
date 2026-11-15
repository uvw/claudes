package cli

import (
	"strings"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

type planRow struct {
	id, name, label, cwd, note string
}

func (app *App) printPlan(heading string, rows []planRow) {
	app.printPlanOf(heading, "session(s)", rows)
}

// printPlanOf is printPlan for rows that are not sessions, such as folders.
func (app *App) printPlanOf(heading, noun string, rows []planRow) {
	app.say("%s %d %s:", heading, len(rows), noun)
	for _, r := range rows {
		app.record(func(rep *report) {
			rep.Plan = append(rep.Plan, reportEntry{ID: r.id, Name: r.name, Cwd: r.cwd, Result: r.label, Note: r.note})
		})
		note := ""
		if r.note != "" {
			note = "  (" + r.note + ")"
		}
		app.say("  %-8s  %-26s  %-8s  %s%s", r.id, r.name, r.label, homeShort(r.cwd), note)
	}
}

// gate ends a dry run once its plan is printed, and asks before going ahead
// with --confirm. proceed is false when the command should stop; err is
// errReported when the user said no.
func (app *App) gate(f *lifecycleFlags, question string) (proceed bool, err error) {
	if f.dryRun {
		app.say("\nDry run: nothing was changed.")
		return false, nil
	}
	if f.confirm && !app.confirmed("\n"+question) {
		app.say("Cancelled.")
		return false, errReported
	}
	return true, nil
}

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
				e.Note = why
				rep.LeftOut = append(rep.LeftOut, e)
			} else {
				rep.Manual = append(rep.Manual, e)
			}
		})
	}
}

// printLeftOut names the sessions a bulk command did not act on, and why.
func (app *App) printLeftOut(sel selection, verb string) {
	printCommands(app, "Interactive sessions were left alone (--interactive stops them). "+
		"To restart one, run this in its terminal:", "interactive", sel.leftAlone)
	for _, group := range []struct {
		format, why string
		sessions    []claude.Session
	}{
		{"Pinned, not %s: %s. Name a session to include it.", "pinned", sel.pinned},
		{"Leading an agent team, not %s: %s. Its teammates would stop with it and cannot be resumed; name it to include it anyway.", "leads a team", sel.leads},
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
			rep.Failures = append(rep.Failures, reportEntry{Name: f.label, Error: f.reason})
		})
	}
	return errSomeFailed
}
