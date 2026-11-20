package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/history"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/transcripts"
)

type gcFlags struct {
	olderThan                                         days
	empty, trivial, gone, copies, leftovers, sidecars bool
	planFlags
	purge bool
}

func (f gcFlags) kinds() history.Kinds {
	return history.Kinds{Empty: f.empty, Trivial: f.trivial, Gone: f.gone, Copies: f.copies, Leftovers: f.leftovers,
		Sidecars: f.sidecars, OlderThan: f.olderThan.d}
}

func (app *App) sessionGcCommand() *cobra.Command {
	f := gcFlags{olderThan: app.daysFlag(0)}
	cmd := &cobra.Command{
		Use:   "gc [TARGET...]",
		Short: "Remove past sessions that are empty, gone, old, or left over",
		Long: `Remove past sessions from ~/.claude, with everything Claude Code keeps for each:
its transcript, subagent transcripts and tool results, older copies, file
history, session-env directory, and debug log. They go to the system trash, so a
mistake can be undone; --purge deletes them for good. Windows deletes an item
too big for the Recycle Bin, or on a drive without one, for good.

Without flags or targets, removes:

  empty      sessions that never got a prompt
  copies     older copies session mv left, keeping the newest
  leftovers  subagent and file-history directories whose transcript is gone

--empty, --trivial, --gone, --copies, --leftovers, and --older-than pick exactly
the kinds they name instead. --trivial takes sessions that changed no file and
made no tool call, and either only ran slash commands or got one prompt, such as
a quick health check; a session renamed with /rename or started by an agent team
is never trivial. --gone takes sessions whose directory no longer exists, which
includes a directory on a disk that is not attached, so read the plan first.
--older-than takes sessions last active longer ago than that.

--sidecars keeps each session and trims its sidecar directory instead: the big
command outputs Claude Code saved to tool-results, which the transcript keeps a
preview of. With --older-than, it also takes the subagent transcripts of
sessions last active longer ago than that, and removes no whole session. A
session resumes as before; it only cannot read a saved output again, or continue
a subagent it ran.

It never removes a running session, one agent view still lists (claudes gc
removes those from the list), a parked one, or the session running claudes.
Without targets, it also leaves alone anything that changed in the last day.
Targets remove exactly those sessions, whatever kind they are.`,
		ValidArgsFunction: app.completePast(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.sessionGc(cmd.Context(), args, f)
		},
	}
	cmd.Flags().BoolVar(&f.empty, "empty", false, "sessions that never got a prompt")
	cmd.Flags().BoolVar(&f.trivial, "trivial", false, "sessions that only ran slash commands, or got one prompt and made no tool call")
	cmd.Flags().BoolVar(&f.gone, "gone", false, "sessions whose directory no longer exists")
	cmd.Flags().BoolVar(&f.copies, "copies", false, "older copies session mv left")
	cmd.Flags().BoolVar(&f.sidecars, "sidecars", false, "saved command outputs, and with --older-than subagent transcripts, keeping the sessions")
	cmd.Flags().BoolVar(&f.leftovers, "leftovers", false, "subagent and file-history directories whose transcript is gone")
	cmd.Flags().Var(&f.olderThan, "older-than", "sessions last active longer ago than this, such as 90d or month")
	cmd.Flags().Lookup("older-than").DefValue = ""
	cmd.Flags().BoolVar(&f.purge, "purge", false, "delete for good instead of moving to the trash")
	f.planFlags.add(cmd)
	return cmd
}

func (app *App) sessionGc(ctx context.Context, targets []string, f gcFlags) error {
	if len(targets) > 0 && (f.empty || f.trivial || f.gone || f.copies || f.leftovers || f.sidecars || f.olderThan.d > 0) {
		return errors.New("session gc takes targets or kinds to remove, not both")
	}
	if !f.empty && !f.trivial && !f.gone && !f.copies && !f.leftovers && !f.sidecars && f.olderThan.d == 0 {
		f.empty, f.copies, f.leftovers = true, true, true
	}
	list, err := app.pastSessions(ctx)
	if err != nil {
		return err
	}
	if !app.runningKnown {
		return refusal(errors.New("cannot tell which sessions are running, so session gc removes nothing"))
	}
	keep, err := app.gcKeep(ctx, list)
	if err != nil {
		return err
	}

	var items []history.Item
	if len(targets) > 0 {
		picked, err := target.Resolve(targets, list)
		if err != nil {
			return err
		}
		for _, p := range picked {
			if why, ok := keep[p.SessionID]; ok {
				return refusal(fmt.Errorf("%s %s, so session gc leaves it alone", p.Label(), why))
			}
			items = append(items, history.Whole(app.Transcripts, p, history.KindNamed))
		}
	} else {
		if items, err = history.Collect(app.Transcripts, list, keep, f.kinds(), app.Now()); err != nil {
			return err
		}
	}
	if len(items) == 0 {
		app.say("Nothing to clean up.")
		return nil
	}

	var total int64
	rows := make([]render.PlanRow, 0, len(items))
	for _, it := range items {
		rows = append(rows, render.PlanRow{ID: it.ID(), Name: it.Name, Label: string(it.Why), Cwd: it.Dir(), Size: render.Size(it.Size)})
		total += it.Size
	}
	app.printPlanOf("Removing", "session(s) and dir(s)", "WHY", rows)
	where := "Moves them to the trash; --purge deletes them for good."
	if f.purge {
		where = "Deletes them for good."
	}
	app.say("\nFrees %s. %s", render.Size(total), where)
	question := "Move these to the trash?"
	if f.purge {
		question = "Delete these for good?"
	}
	if proceed, err := app.gate(f.planFlags, question); !proceed {
		return err
	}
	return app.gcRemove(ctx, items, f.purge)
}

// gcRemove removes items, after checking once more that none of them runs
// and that none changed since the plan.
func (app *App) gcRemove(ctx context.Context, items []history.Item, purge bool) error {
	running, err := app.Agents.Sessions(ctx, false)
	if err != nil {
		return refusal(fmt.Errorf("cannot tell which sessions are running, so session gc removes nothing: %w", err))
	}
	live := map[string]bool{}
	for _, s := range running {
		if s.Running() {
			live[s.SessionID] = true
		}
	}
	verb := "trashed"
	if purge {
		verb = "deleted"
	}
	var failures []render.Failure
	var freed int64
	for i, it := range items {
		if ctx.Err() != nil {
			notReached(app, items[i:])
			break
		}
		if live[it.SessionID] && it.Why != history.KindLeftover {
			app.progress("skipped", it, " (it started running)")
			continue
		}
		if it.Transcript != "" && !transcripts.Changed(it.Transcript).Equal(it.Changed) {
			app.progress("skipped", it, " (it changed since the plan)")
			continue
		}
		if err := app.Transcripts.Remove(it.SessionID, it.Paths, app.remover(purge)); err != nil {
			failures = append(failures, render.Failure{Label: it.Name, Reason: err.Error(), Of: it})
			app.progressFailed(it)
			continue
		}
		if len(it.Copies) > 0 {
			if err := app.Transcripts.ForgetMoves(it.SessionID, it.Copies...); err != nil {
				app.warn("claudes: removed %s, but cannot update moves.json: %v", it.Name, err)
			}
		}
		freed += it.Size
		app.progress(verb, it, " ("+render.Size(it.Size)+")")
	}
	if freed > 0 {
		app.say("\nFreed %s.", render.Size(freed))
	}
	return app.reportFailures(failures)
}
