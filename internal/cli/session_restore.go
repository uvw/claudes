package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/trash"
)

func (app *App) sessionRestoreCommand() *cobra.Command {
	var f planFlags
	var out render.Format
	cmd := &cobra.Command{
		Use:   "restore [TARGET...]",
		Short: "Put back sessions claudes moved to the trash",
		Long: `Put back the files claudes moved to the system trash for each session named,
each to where it came from. That covers what session gc, session mv --rm, and
gc --teams moved there. Without targets, list the sessions it can put back.

It refuses, and puts nothing back, when a file's original path is taken, since
it never replaces a file, or when a file is no longer in the trash, such as
after the trash was emptied. A file it cannot move stays in the trash, and
running restore again tries it again.

claudes records where each file went in trashed.json, in its state directory.
--purge records nothing. Windows does not say where in the Recycle Bin a file
went, so there, put it back from the Recycle Bin itself.`,
		ValidArgsFunction: func(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			entries, _ := app.Trashed.Entries()
			var out []cobra.Completion
			for _, e := range entries {
				out = append(out, cobra.CompletionWithDesc(e.Label(), e.ID()+" trashed "+render.Ago(app.Now(), e.Trashed)))
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		Annotations: map[string]string{listsBare: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return app.listTrashed(out)
			}
			return app.sessionRestore(args, f)
		},
	}
	f.add(cmd)
	app.addOutput(cmd, &out, "")
	return cmd
}

// listTrashed lists the sessions session restore can put back.
func (app *App) listTrashed(out render.Format) error {
	entries, err := app.Trashed.Entries()
	if err != nil {
		return err
	}
	if out != render.Text {
		if entries == nil {
			entries = []trash.Entry{}
		}
		return out.Encode(app.Out, entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(app.Out, "Nothing to restore.")
		if trash.SystemRestore != "" {
			fmt.Fprintln(app.Out, trash.SystemRestore)
		}
		return nil
	}
	return app.writeTable(app.Out, render.PlanTable("TRASHED", app.trashedRows(entries)), tableFit{})
}

// trashedRows are entries as a table shows them.
func (app *App) trashedRows(entries []trash.Entry) []render.PlanRow {
	rows := make([]render.PlanRow, 0, len(entries))
	for _, e := range entries {
		row := render.PlanRow{ID: e.ID(), Name: e.Label(), Label: render.Ago(app.Now(), e.Trashed), Size: render.Size(e.Size), Cwd: e.Cwd}
		if e.Missing > 0 {
			row.Note = fmt.Sprintf("%d file(s) no longer in the trash", e.Missing)
		}
		rows = append(rows, row)
	}
	return rows
}

func (app *App) sessionRestore(targets []string, f planFlags) error {
	entries, err := app.Trashed.Entries()
	if err != nil {
		return err
	}
	picked, err := target.Resolve(targets, entries)
	if err != nil {
		if trash.SystemRestore != "" {
			return fmt.Errorf("%w\n%s", err, trash.SystemRestore)
		}
		return err
	}
	ids := make([]string, len(picked))
	for i, e := range picked {
		ids[i] = e.ID()
	}
	files, problems := app.Trashed.Plan(ids)
	app.printPlanOf("Restoring", "session(s)", "TRASHED", app.trashedRows(picked))
	app.say("\nPuts back %d file(s):", len(files))
	for _, file := range files {
		app.say("  %s", file.From)
	}
	if len(problems) > 0 {
		return app.restoreRefused(&trash.Blocked{Problems: problems})
	}
	if proceed, err := app.gate(f, "Put these back?"); !proceed {
		return err
	}
	res, err := app.Trashed.Restore(ids)
	var blocked *trash.Blocked
	if errors.As(err, &blocked) {
		return app.restoreRefused(blocked)
	}
	if len(res.Back) == 0 && len(res.Failed) == 0 && err != nil {
		return err
	}
	for _, moves := range res.Moves {
		if merr := app.Transcripts.RememberMoves(moves); merr != nil {
			app.warn("claudes: put back a moved session, but cannot update moves.json: %v", merr)
		}
	}
	app.say("")
	var failures []render.Failure
	for _, e := range picked {
		back, stayed := 0, []string{}
		for _, b := range res.Back {
			if e.Holds(b.From) {
				back++
			}
		}
		for _, fail := range res.Failed {
			if e.Holds(fail.From) {
				stayed = append(stayed, fmt.Sprintf("%s stayed in the trash at %s: %v", fail.From, fail.To, fail.Err))
			}
		}
		if len(stayed) == 0 {
			app.progress("restored", e, fmt.Sprintf(" (%d file(s))", back))
			continue
		}
		app.progressFailed(e)
		failures = append(failures, render.Failure{Label: e.Label(), Of: e,
			Reason: fmt.Sprintf("put back %d of %d file(s). %s. Run session restore %s again to retry", back, back+len(stayed), strings.Join(stayed, "; "), e.ID())})
	}
	if err != nil {
		app.warn("claudes: put the files back, but cannot update %s, so it still lists them: %v", app.Trashed.Path, err)
	}
	return app.reportFailures(failures)
}

// restoreRefused is the refusal when session restore puts nothing back, with
// every reason on a line of its own.
func (app *App) restoreRefused(b *trash.Blocked) error {
	return refusal(fmt.Errorf("session restore put nothing back:\n  %s", strings.ReplaceAll(b.Error(), "\n", "\n  ")))
}
