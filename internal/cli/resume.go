package cli

import (
	"context"
	"errors"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
)

// resumable is a session resume can bring back: a parked entry, or a stopped
// session named as a target.
type resumable struct {
	park.Entry
	parked bool
}

func (app *App) resumeCommand() *cobra.Command {
	var f lifecycleFlags
	cmd := &cobra.Command{
		Use:   "resume [TARGET...]",
		Short: "Bring paused sessions back, or recover stopped ones by name",
		Long: `Bring back the sessions claudes pause stopped. With targets, resumes those:
parked sessions, or any stopped or failed session by name, such as one killed by
accident.

--ping sends a prompt to parked sessions that were mid-turn when paused, and to
every named session that was not parked.

An interactive session stays parked until it is seen running again, so its
command is printed by every resume until then. --forget drops it instead.`,
		ValidArgsFunction: app.completeResumable,
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.forget {
				return app.forget(args, f.dryRun)
			}
			return app.resume(cmd.Context(), args, &f)
		},
	}
	f.addCommon(cmd, app.completeResumeSkip)
	f.addPing(cmd, app.pingPrompt(defaultPing))
	app.addForce(cmd, forceUsage)
	cmd.Flags().BoolVar(&f.interactiveBG, "interactive-bg", false, "resume interactive sessions in the background, not print commands")
	cmd.Flags().BoolVar(&f.forget, "forget", false, "drop the named sessions from the park list without resuming them")
	return cmd
}

func (app *App) forget(targets []string, dryRun bool) error {
	if len(targets) == 0 {
		return errors.New("--forget needs the sessions to drop")
	}
	if dryRun {
		entries, err := app.Park.Load()
		if err != nil {
			return err
		}
		picked, err := target.Resolve(targets, entries)
		if err != nil {
			return err
		}
		for _, e := range picked {
			app.say("Would drop from the park list: %s", e.Label())
			app.record(func(r *report) { r.Plan = append(r.Plan, render.EntryOf(e)) })
		}
		app.dryRunDone()
		return nil
	}
	var dropped []park.Entry
	var resolveErr error
	err := app.Park.Update(func(entries []park.Entry) []park.Entry {
		if dropped, resolveErr = target.Resolve(targets, entries); resolveErr != nil {
			return entries
		}
		return slices.DeleteFunc(entries, func(e park.Entry) bool {
			return slices.ContainsFunc(dropped, func(p park.Entry) bool { return p.SessionID == e.SessionID })
		})
	})
	if err = errors.Join(resolveErr, err); err != nil {
		return err
	}
	for _, e := range dropped {
		app.say("Dropped from the park list: %s", e.Label())
		app.record(func(r *report) {
			x := render.EntryOf(e)
			x.Result = "forgotten"
			r.Results = append(r.Results, x)
		})
	}
	return nil
}

func (app *App) resume(ctx context.Context, targets []string, f *lifecycleFlags) error {
	text := f.text()
	if err := app.Agents.CheckPrompt(text); err != nil {
		return err
	}
	entries, err := app.Park.Load()
	if err != nil {
		return err
	}
	if len(targets) == 0 && len(entries) == 0 {
		app.say("Nothing is parked.")
		return nil
	}
	listed, err := app.listed(ctx, true)
	if err != nil {
		return err
	}
	running := map[string]bool{}
	for _, s := range listed {
		if s.Running() {
			running[s.SessionID] = true
		}
	}

	candidates := make([]resumable, 0, len(entries))
	parkedIDs := map[string]bool{}
	for _, e := range entries {
		candidates = append(candidates, resumable{Entry: e, parked: true})
		parkedIDs[e.SessionID] = true
	}
	picked := candidates
	if len(targets) > 0 {
		for _, s := range listed {
			if !parkedIDs[s.SessionID] {
				e := park.FromSession(s, true, "")
				candidates = append(candidates, resumable{Entry: e})
			}
		}
		if picked, err = target.Resolve(targets, candidates); err != nil {
			return err
		}
	}

	skip, err := skipSet(f.skip, candidates, func(r resumable) string { return r.SessionID })
	if err != nil {
		return err
	}
	var todo, already, skippedHere []resumable
	for _, r := range picked {
		switch {
		case running[r.SessionID]:
			already = append(already, r)
		case skip[r.SessionID]:
			skippedHere = append(skippedHere, r)
		default:
			todo = append(todo, r)
		}
	}

	printsCommand := func(r resumable) bool { return r.Interactive() && !f.interactiveBG }
	promptFor := func(r resumable) string {
		if r.WasBusy {
			return text
		}
		return ""
	}
	if len(todo) > 0 {
		rows := make([]render.PlanRow, 0, len(todo))
		for _, r := range todo {
			label, note := "idle", ""
			if r.WasBusy && r.parked {
				label = "was busy"
			}
			if !r.parked {
				label = "stopped"
			}
			switch {
			case printsCommand(r):
				note = "interactive: prints its command"
			case promptFor(r) != "":
				note = "will be pinged"
			}
			rows = append(rows, render.PlanRow{ID: r.ShortID(), Name: r.Label(), Label: label, Cwd: r.Cwd, Note: note})
		}
		app.printPlan("Resuming", rows)
	}
	for _, r := range already {
		if r.parked {
			app.say("Already running, dropped from the park list: %s", r.Label())
		} else {
			app.say("Already running: %s", r.Label())
		}
	}
	if len(skippedHere) > 0 {
		app.say("Skipped, still parked: %s", render.Labels(skippedHere))
	}
	if len(todo) > 0 || f.dryRun {
		if proceed, err := app.gate(f.planFlags, "Resume these sessions?"); !proceed {
			return err
		}
	}

	// Parked entries stay unless they are resumed or already running. An
	// interactive one stays until it is seen running, since its command is
	// only printed here.
	done := map[string]bool{}
	for _, r := range already {
		done[r.SessionID] = true
	}
	hand := context.WithoutCancel(ctx)
	var failures []render.Failure
	var manual []resumable
	for i, r := range todo {
		if ctx.Err() != nil {
			notReached(app, todo[i:])
			break
		}
		if printsCommand(r) {
			manual = append(manual, r)
			continue
		}
		prompt := promptFor(r)
		err := app.resumeSession(hand, r.SessionID, r.Cwd, prompt)
		switch {
		case errors.Is(err, errNotYetRunning):
			// Kept parked, so a second claudes resume finds it if it never
			// came up; one that did is skipped as running.
			app.warn("claudes: %s: %v.", r.Label(), err)
		case err != nil:
			failures = append(failures, render.Failure{Label: r.Label(), Reason: err.Error(), Of: r})
			app.progressFailed(r)
			continue
		default:
			done[r.SessionID] = true
		}
		if prompt != "" {
			app.progress("resumed", r, " and pinged")
		} else {
			app.progress("resumed", r, "")
		}
	}

	printManual(app, "Resume each interactive session in its own terminal:", manual)
	var kept []park.Entry
	err = app.Park.Update(func(entries []park.Entry) []park.Entry {
		kept = slices.DeleteFunc(entries, func(e park.Entry) bool { return done[e.SessionID] })
		return kept
	})
	if err != nil {
		failures = append(failures, render.Failure{Label: "park file", Reason: err.Error()})
	}
	if len(kept) > 0 {
		app.say("\n%d session(s) still parked in %s.", len(kept), app.Park.Path)
	}
	return app.reportFailures(failures)
}
