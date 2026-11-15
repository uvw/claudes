package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
)

func (app *App) pauseCommand() *cobra.Command {
	var f lifecycleFlags
	cmd := &cobra.Command{
		Use:   "pause [TARGET...]",
		Short: "Stop running sessions and remember them for claudes resume",
		Long: `Stop running sessions and remember them in the park file, so claudes resume can
bring them back, after a reboot if need be. With targets, pauses exactly those.
Without, pauses every running background session except pinned ones.`,
		ValidArgsFunction: app.completeSessions(runningSession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.pause(cmd.Context(), args, &f)
		},
	}
	f.addCommon(cmd, app.completeSkip(runningSession))
	f.addWait(cmd, app.waitTimeout())
	f.addInteractive(cmd, "also stop interactive sessions", "")
	app.addForce(cmd, forceUsage)
	return cmd
}

func (app *App) pause(ctx context.Context, targets []string, f *lifecycleFlags) error {
	sel, err := app.acquire(ctx, targets, f)
	if err != nil {
		return err
	}
	if len(sel.chosen) == 0 {
		app.say("No running sessions to pause.")
		app.printLeftOut(sel, "paused")
		return nil
	}
	rows := make([]planRow, 0, len(sel.chosen))
	for _, s := range sel.chosen {
		note := ""
		if app.isHost(s) {
			note = "this session: paused last"
		}
		note = joinNotes(note, app.teamNote(s))
		rows = append(rows, planRow{s.ID(), s.Label(), dash(s.Activity()), s.Cwd, note})
	}
	app.printPlan("Pausing", rows)
	if proceed, err := app.gate(f, "Pause these sessions?"); !proceed {
		if err == nil {
			app.printLeftOut(sel, "paused")
		}
		return err
	}

	stamp := app.Now().UTC().Format(time.RFC3339)
	added := make([]park.Entry, 0, len(sel.chosen))
	for _, s := range sel.chosen {
		added = append(added, park.FromSession(s, stamp))
	}
	// The list goes to disk before anything stops, so a crash cannot lose it.
	if err := app.Park.Update(func(existing []park.Entry) []park.Entry { return park.Merge(existing, added) }); err != nil {
		return err
	}

	others, host := app.splitHost(sel.chosen)
	hand := context.WithoutCancel(ctx)
	var failures []failure
	for i, s := range others {
		if ctx.Err() != nil {
			notReached(app, others[i:])
			break
		}
		if !claude.FolderExists(s.Cwd) {
			failures = append(failures, failure{s.Label(), fmt.Sprintf("folder %s is gone, so it was not stopped: it could not be resumed", s.Cwd), s})
			if err := app.unpark(s.SessionID); err != nil {
				failures = append(failures, failure{label: "park file", reason: err.Error()})
			}
			app.progressFailed(s)
			continue
		}
		if err := app.stopSession(hand, s); err != nil {
			failures = append(failures, failure{s.Label(), app.stopFailed(hand, s, err).Error(), s})
			app.progressFailed(s)
			continue
		}
		app.progress("paused", s, "")
	}
	if parked := app.countParked(sel.chosen); parked > 0 {
		app.say("\nParked %d session(s) in %s.", parked, app.Park.Path)
		app.say("Bring them back with: claudes resume")
	}
	app.printLeftOut(sel, "paused")
	result := app.reportFailures(failures)
	if host != nil && ctx.Err() == nil {
		if err := app.stopHost(ctx, *host, "Pausing", true); err != nil {
			app.warn("claudes: this session (%s): %v", host.Label(), err)
			return errSomeFailed
		}
	}
	return result
}
