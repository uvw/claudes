package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/history"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/trash"
)

type gcFlags struct {
	olderThan                                                   days
	empty, trivial, headless, gone, copies, leftovers, sidecars bool
	// junk takes the default kinds, trivial sessions, and headless ones.
	junk bool
	planFlags
	purge bool
}

func (f gcFlags) kinds() history.Kinds {
	return history.Kinds{Empty: f.empty, Trivial: f.trivial, Headless: f.headless, Gone: f.gone, Copies: f.copies,
		Leftovers: f.leftovers, Sidecars: f.sidecars, OlderThan: f.olderThan.d}
}

// anyKind reports whether a flag names a kind to remove.
func (f gcFlags) anyKind() bool {
	return f.empty || f.trivial || f.headless || f.junk || f.gone || f.copies || f.leftovers || f.sidecars || f.olderThan.d > 0
}

func (app *App) sessionGcCommand() *cobra.Command {
	f := gcFlags{olderThan: app.daysFlag(0)}
	cmd := &cobra.Command{
		Use:   "gc [TARGET...]",
		Short: "Remove past sessions that are empty, gone, old, or left over",
		Long: `Remove past sessions from ~/.claude, with everything Claude Code keeps for each:
its transcript, subagent transcripts and tool results, older copies, file
history, session-env directory, and debug log. They go to the system trash, and
claudes session restore puts them back; --purge deletes them for good. Windows
deletes an item too big for the Recycle Bin, or on a drive without one, for
good.

Without flags or targets, removes:

  empty      sessions that never got a prompt
  copies     older copies session mv left, keeping the newest
  leftovers  subagent and file-history directories whose transcript is gone

--empty, --trivial, --headless, --gone, --copies, --leftovers, and --older-than
pick exactly the kinds they name instead. --trivial takes sessions that changed
no file and made no tool call, and either only ran slash commands or got one
prompt, such as a quick health check. --headless takes one-shots a program ran
with claude -p or the Agent SDK and nobody opened in a terminal, such as a tool
that writes commit messages: one prompt, any tool calls, and no file changed.
A session renamed with /rename or started by an agent team is never trivial or
headless. --gone takes sessions whose directory no longer exists, which
includes a directory on a disk that is not attached, so read the plan first.
--older-than takes sessions last active longer ago than that.

--junk takes everything that holds nothing to resume: the kinds above that
session gc takes without flags, and --trivial and --headless. Without flags,
session gc says what --junk would add.

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
	cmd.Flags().BoolVar(&f.headless, "headless", false, "one-prompt sessions claude -p or the Agent SDK ran, that changed no file")
	cmd.Flags().BoolVar(&f.junk, "junk", false, "everything that holds nothing to resume: the default kinds, --trivial, and --headless")
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
	if len(targets) > 0 && f.anyKind() {
		return errors.New("session gc takes targets or kinds to remove, not both")
	}
	// Without flags, it also counts what --junk would add, to say so.
	plain := len(targets) == 0 && !f.anyKind()
	if plain || f.junk {
		f.empty, f.copies, f.leftovers = true, true, true
	}
	if f.junk {
		f.trivial, f.headless = true, true
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
	hint := ""
	if plain {
		if hint, err = app.junkHint(list, keep); err != nil {
			return err
		}
	}
	if len(items) == 0 {
		app.say("Nothing to clean up.")
		if hint != "" {
			app.say("%s", hint)
		}
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
	if hint != "" {
		app.say("%s", hint)
	}
	question := "Move these to the trash?"
	if f.purge {
		question = "Delete these for good?"
	}
	if proceed, err := app.gate(f.planFlags, question); !proceed {
		return err
	}
	return app.gcRemove(ctx, items, f.purge)
}

// junkHint is the line plain session gc adds about what --junk would take
// besides, or "" when that is nothing.
func (app *App) junkHint(list []history.Session, keep map[string]string) (string, error) {
	extra, err := history.Collect(app.Transcripts, list, keep, history.Kinds{Trivial: true, Headless: true}, app.Now())
	if err != nil || len(extra) == 0 {
		return "", err
	}
	count := map[history.Kind]int{}
	var size int64
	for _, it := range extra {
		count[it.Why]++
		size += it.Size
	}
	var parts []string
	for _, k := range []history.Kind{history.KindTrivial, history.KindHeadless} {
		if count[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count[k], k))
		}
	}
	return fmt.Sprintf("Also %s session(s) (%s): add --junk to take them too.",
		strings.Join(parts, " and "), render.Size(size)), nil
}

// gcRemove removes items, after checking once more that none of them runs
// and that none changed since the plan.
func (app *App) gcRemove(ctx context.Context, items []history.Item, purge bool) error {
	running, err := app.listIn(ctx, app.Accounts, false)
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
		rec := trash.Record{SessionID: it.SessionID, Name: it.Name, Cwd: it.Dir()}
		err := app.Transcripts.Remove(it.SessionID, it.Paths, app.remover(purge, &rec))
		if err == nil && len(it.Copies) > 0 {
			var ferr error
			if rec.Moves, ferr = app.Transcripts.ForgetMoves(it.SessionID, it.Copies...); ferr != nil {
				app.warn("claudes: removed %s, but cannot update moves.json: %v", it.Name, ferr)
			}
		}
		app.keepTrashed("session gc", rec)
		if err != nil {
			failures = append(failures, render.Failure{Label: it.Name, Reason: err.Error(), Of: it})
			app.progressFailed(it)
			continue
		}
		freed += it.Size
		app.progress(verb, it, " ("+render.Size(it.Size)+")")
	}
	if freed > 0 {
		app.say("\nFreed %s.", render.Size(freed))
	}
	return app.reportFailures(failures)
}
