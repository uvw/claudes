package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/teams"
)

var finishedStates = []string{"done", "stopped", "failed"}

// isFinished reports whether gc may remove s: a background session that is
// done, stopped, or failed, with no process.
func isFinished(s claude.Session) bool {
	return !s.Running() && s.JobID != "" && slices.Contains(finishedStates, s.State)
}

// requireFinished refuses a named session gc may not remove.
func requireFinished(sessions []claude.Session) error {
	for _, s := range sessions {
		switch {
		case s.Interactive():
			return exit.Refusal(fmt.Errorf("%s is an interactive session: gc removes only finished background sessions", s.Label()))
		case s.Running():
			return exit.Refusal(fmt.Errorf("%s is still running: gc removes only finished background sessions. Stop it first", s.Label()))
		case !isFinished(s):
			return exit.Refusal(fmt.Errorf("%s is not finished (%s): gc removes only finished background sessions", s.Label(), dash(s.State)))
		}
	}
	return nil
}

func (app *App) gcCommand() *cobra.Command {
	var olderThan days
	var plan planFlags
	var withTeams, withPinned, purge bool
	olderThan.d = 7 * 24 * time.Hour
	cmd := &cobra.Command{
		Use:   "gc [TARGET...]",
		Short: "Remove finished background sessions from the list",
		Long: `Remove finished background sessions (done, stopped, or failed, with no process)
from the list with claude rm, which keeps each conversation and deletes its
worktree only when that is safe. With targets, removes those. Without, removes
every finished session last active longer ago than --older-than, except parked
ones, and pinned ones unless --pinned is given.

--teams removes the agent team directories under ~/.claude/teams and the task
lists under ~/.claude/tasks that Claude Code leaves behind: those of sessions
that no longer run and have not changed for longer than --older-than. They go to
the system trash; --purge deletes them for good.`,
		ValidArgsFunction: app.completeSessions(finishedSession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if purge && !withTeams {
				return errors.New("--purge goes with --teams: claude rm keeps each conversation")
			}
			if withTeams {
				if len(args) > 0 {
					return errors.New("gc --teams takes no targets")
				}
				return app.gcTeams(ctx, gcTeamsOpts{olderThan: olderThan.d, plan: plan, withPinned: withPinned, purge: purge})
			}
			sessions, err := app.listed(ctx, true)
			if err != nil {
				return err
			}
			finished := slices.DeleteFunc(slices.Clone(sessions), func(s claude.Session) bool { return !isFinished(s) })
			var picked []claude.Session
			if len(args) > 0 {
				if picked, err = target.Resolve(args, sessions); err == nil {
					err = requireFinished(picked)
				}
			} else {
				picked, err = app.oldFinished(finished, app.Now().Add(-olderThan.d), withPinned)
			}
			if err != nil {
				return err
			}
			if len(picked) == 0 {
				app.say("Nothing to clean up.")
				return nil
			}
			rows := make([]planRow, 0, len(picked))
			for _, s := range picked {
				rows = append(rows, planRow{id: s.ID(), name: s.Label(), label: s.State, cwd: s.Cwd, note: "last active " + ago(app.Now(), app.lastActive(s))})
			}
			app.printPlanOf("Removing", "session(s)", "STATE", rows)
			if proceed, err := app.gate(plan, "Remove these sessions from the list?"); !proceed {
				return err
			}
			hand := context.WithoutCancel(ctx)
			var failures []failure
			for i, s := range picked {
				if ctx.Err() != nil {
					notReached(app, picked[i:])
					break
				}
				if err := app.Agents.Remove(hand, s.JobID); err != nil {
					failures = append(failures, failure{s.Label(), err.Error(), s})
					app.progressFailed(s)
					continue
				}
				app.progress("removed", s, "")
			}
			return app.reportFailures(failures)
		},
	}
	cmd.Flags().Var(&olderThan, "older-than", "without targets, only sessions older than this, such as 7d or 12h")
	cmd.Flags().BoolVar(&withTeams, "teams", false, "remove leftover agent team directories and task lists instead of sessions")
	cmd.Flags().BoolVar(&withPinned, "pinned", false, "without targets, remove pinned sessions too")
	cmd.Flags().BoolVar(&purge, "purge", false, "with --teams, delete for good instead of moving to the trash")
	plan.add(cmd)
	app.addForce(cmd, forceUsage)
	return cmd
}

// oldFinished is the finished sessions last active before cutoff, except
// parked ones, and pinned ones unless withPinned.
func (app *App) oldFinished(finished []claude.Session, cutoff time.Time, withPinned bool) ([]claude.Session, error) {
	pins := map[string]bool{}
	if !withPinned {
		var err error
		if pins, err = app.protectedPins(finished); err != nil {
			return nil, err
		}
	}
	entries, err := app.Park.Load()
	if err != nil {
		return nil, fmt.Errorf("cannot read the park file, so cannot tell which sessions are paused: %w", err)
	}
	var out []claude.Session
	for _, s := range finished {
		isParked := slices.ContainsFunc(entries, func(e park.Entry) bool { return e.SessionID == s.SessionID })
		last := app.lastActive(s)
		if !pins[s.JobID] && !isParked && !last.IsZero() && last.Before(cutoff) {
			out = append(out, s)
		}
	}
	return out, nil
}

// lastActive is when a session last changed state: its state.json, or when it
// started if that is unknown.
func (app *App) lastActive(s claude.Session) time.Time {
	if t := app.Jobs.LastActive(s.JobID); !t.IsZero() {
		return t
	}
	return s.Started()
}

// leftover is a team or task-list directory gc --teams removes.
type leftover struct{ teams.Folder }

func (l leftover) ID() string { return strings.TrimPrefix(l.Name, "session-") }

func (l leftover) Label() string {
	if filepath.Base(filepath.Dir(l.Path)) == "teams" {
		return "team"
	}
	return "task list"
}

// gcTeamsOpts are the flags of gc --teams.
type gcTeamsOpts struct {
	olderThan         time.Duration
	plan              planFlags
	withPinned, purge bool
}

// gcTeams removes the team directories and task lists of sessions that no longer
// run and that have not changed since olderThan ago, keeping a pinned
// session's unless withPinned.
func (app *App) gcTeams(ctx context.Context, o gcTeamsOpts) error {
	sessions, err := app.listed(ctx, true)
	if err != nil {
		return err
	}
	// Kept: a running session's, and one claudes resume or a pin brings back.
	// A pinned job id is its session's short id, so a pin keeps its team even
	// when claude agents no longer lists the session.
	keep := map[string]bool{}
	pinned := map[string]bool{}
	if !o.withPinned {
		if pinned, err = app.protectedPins(sessions); err != nil {
			return err
		}
		maps.Copy(keep, pinned)
	}
	for _, s := range sessions {
		if s.Running() || pinned[s.JobID] {
			keep[claude.ShortID(s.SessionID)] = true
		}
	}
	parked, err := app.Park.Load()
	if err != nil {
		return err
	}
	for _, e := range parked {
		keep[claude.ShortID(e.SessionID)] = true
	}
	folders, err := app.Teams.Leftovers(keep, app.Now().Add(-o.olderThan))
	if err != nil {
		return err
	}
	if len(folders) == 0 {
		app.say("No team directories or task lists to clean up.")
		return nil
	}
	rows := make([]planRow, 0, len(folders))
	for _, f := range folders {
		l := leftover{f}
		rows = append(rows, planRow{id: l.ID(), name: l.Label(), cwd: f.Path, note: "last changed " + ago(app.Now(), f.Modified)})
	}
	app.printPlanOf("Removing", "team dir(s) and task list(s)", "", rows)
	if proceed, err := app.gate(o.plan, "Remove these team dirs and task lists?"); !proceed {
		return err
	}
	verb := "trashed"
	if o.purge {
		verb = "deleted"
	}
	var failures []failure
	for i, f := range folders {
		if ctx.Err() != nil {
			notReached(app, leftovers(folders[i:]))
			break
		}
		l := leftover{f}
		if err := app.Teams.Remove(f, app.remover(o.purge)); err != nil {
			failures = append(failures, failure{l.Label() + " " + l.ID(), err.Error(), l})
			app.progressFailed(l)
			continue
		}
		app.progress(verb, l, "")
	}
	return app.reportFailures(failures)
}

func leftovers(folders []teams.Folder) []leftover {
	out := make([]leftover, len(folders))
	for i, f := range folders {
		out[i] = leftover{f}
	}
	return out
}
