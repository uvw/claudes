package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/shell"
)

// printPlan prints the sessions a command is about to act on, with their
// status.
func (app *App) printPlan(heading string, rows []render.PlanRow) {
	app.printPlanOf(heading, "session(s)", "STATUS", rows)
}

// printPlanOf is printPlan for rows that are not sessions, such as directories,
// or whose label is not a status.
func (app *App) printPlanOf(heading, noun, labelHeader string, rows []render.PlanRow) {
	app.say("%s %d %s:", heading, len(rows), noun)
	for _, r := range rows {
		app.record(func(rep *report) { rep.Plan = append(rep.Plan, r.Entry()) })
	}
	var b strings.Builder
	_ = app.writeTable(&b, render.PlanTable(labelHeader, rows), tableFit{margin: 2})
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
func (app *App) progress(verb string, s render.Subject, note string) {
	paint := app.colors.Green
	if verb == "skipped" {
		paint = func(text string) string { return text }
	}
	app.say("  %s %s%s", render.Pad(verb, 10, paint), s.Label(), note)
	app.record(func(rep *report) { rep.Results = append(rep.Results, render.Result(s, verb, note)) })
}

// progressFailed prints a session that failed, to stderr.
func (app *App) progressFailed(s render.Subject) {
	app.warn("  %s %s", render.Pad("FAILED", 10, app.errColors.Red), s.Label())
	app.record(func(rep *report) { rep.Results = append(rep.Results, render.Result(s, "failed", "")) })
}

type manualRef interface {
	render.Subject
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
			e := render.EntryOf(r)
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
func (app *App) printLeftOut(sel selection.Selection, verb string) {
	why := "(--interactive stops them)"
	if sel.Outdated {
		why = "(--outdated never picks them)"
	}
	printCommands(app, "Interactive sessions were left alone "+why+". "+
		"Each one resumes with this command, run in its terminal:", "interactive", sel.LeftAlone)
	for _, group := range []struct {
		format, why string
		sessions    []claude.Session
	}{
		{"Pinned, not %s: %s. Add --pinned to include them. Named sessions replace the bulk selection, so name every session you want.", "pinned", sel.Pinned},
		{"Leading an agent team, not %s: %s. Its teammates would stop with it and cannot be resumed. To include it anyway, name it with every other session you want.", "leads a team", sel.Leads},
		{"Skipped, not %s: %s.", "skipped", sel.Skipped},
		{"Not %s, because claude agents gave no job id: %s.", "no job id", sel.NoJob},
	} {
		if len(group.sessions) > 0 {
			app.say("")
			app.say(group.format, verb, render.Labels(group.sessions))
		}
		for _, s := range group.sessions {
			app.record(func(rep *report) { rep.LeftOut = append(rep.LeftOut, render.Result(s, group.why, "")) })
		}
	}
}

func (app *App) reportFailures(failures []render.Failure) error {
	if len(failures) == 0 {
		return nil
	}
	app.warn("")
	app.errorLine("%d session(s) need attention:", len(failures))
	for _, f := range failures {
		app.errorLine("  %s: %s", f.Label, f.Reason)
		app.record(func(rep *report) { rep.Failures = append(rep.Failures, f.Entry()) })
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

// report is the report a command prints with -o json or -o yaml, and how far
// it got.
type report struct {
	render.Report
	format  render.Format
	printed bool
}

// reports adds -o to a command that changes sessions.
func (app *App) reports(cmd *cobra.Command) *cobra.Command {
	var out render.Format
	app.addOutput(cmd, &out, "")
	return cmd
}

// startReport switches a command to a report when it was given -o json or
// -o yaml. Its text output is dropped from then on.
func (app *App) startReport(cmd *cobra.Command) {
	f := cmd.Flags().Lookup("output")
	if f == nil || cmd.Annotations[runsAlone] == "" || f.Value.String() == "text" {
		return
	}
	d := cmd.Flags().Lookup("dry-run")
	app.rep = &report{
		Report: *render.NewReport(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "), d != nil && d.Value.String() == "true"),
		format: render.Format(f.Value.String()),
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
	if err := app.rep.format.Encode(app.Out, &app.rep.Report); err != nil {
		app.warn("claudes: cannot print the report: %v", err)
	}
}
