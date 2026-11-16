package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/version"
)

// selection is what a restart or pause acts on.
type selection struct {
	chosen []claude.Session
	// leftAlone are running interactive sessions nobody asked to touch.
	leftAlone []claude.Session
	// pinned are running sessions skipped because they are pinned.
	pinned []claude.Session
	// skipped are the sessions --skip named.
	skipped []claude.Session
	// noJob are background sessions claude agents gave no job id, which
	// claude cannot stop or respawn.
	noJob []claude.Session
	// leads are sessions skipped because their agent team has teammates at
	// work, which would stop with them and could not be resumed.
	leads []claude.Session
}

// selectOpts are the flags that decide what selectSessions picks.
type selectOpts struct {
	skip            []string
	withInteractive bool
	outdated        bool
	withPinned      bool
}

func (f *lifecycleFlags) selectOpts() selectOpts {
	return selectOpts{skip: f.skip, withInteractive: f.interactive || f.interactiveBG, outdated: f.outdated, withPinned: f.pinned}
}

// selectSessions picks the running sessions a restart or pause acts on. With
// targets it takes exactly those. Without, it takes every running session
// except pinned ones unless asked, and interactive ones unless asked.
func (app *App) selectSessions(ctx context.Context, targets []string, opts selectOpts) (selection, error) {
	// Targets are matched against finished sessions too, so a name that
	// belongs to a stopped session never falls through to a running one whose
	// name merely starts with it.
	sessions, err := app.listed(ctx, len(targets) > 0)
	if err != nil {
		return selection{}, err
	}
	running := slices.DeleteFunc(slices.Clone(sessions), func(s claude.Session) bool { return !s.Running() })
	skip, err := skipSet(opts.skip, running, func(s claude.Session) string { return s.SessionID })
	if err != nil {
		return selection{}, err
	}
	var sel selection
	if len(targets) > 0 {
		sel, err = named(targets, sessions, skip, opts.withInteractive)
	} else {
		sel, err = app.bulk(running, skip, opts.withInteractive, opts.withPinned)
	}
	if err != nil {
		return selection{}, err
	}
	if opts.outdated {
		if err := app.keepOutdated(ctx, &sel); err != nil {
			return selection{}, err
		}
	}
	sel.chosen = app.hostLast(sel.chosen)
	return sel, nil
}

// named is the selection for explicit targets: exactly those sessions.
func named(targets []string, sessions []claude.Session, skip map[string]bool, withInteractive bool) (selection, error) {
	var sel selection
	picked, err := target.Resolve(targets, sessions)
	if err != nil {
		return sel, err
	}
	if err := requireRunning(picked); err != nil {
		return sel, err
	}
	for _, s := range picked {
		if s.Interactive() && !withInteractive {
			return sel, refused(fmt.Errorf("%s is an interactive session: add --interactive or --interactive-bg", s.Label()))
		}
		if skip[s.SessionID] {
			sel.skipped = append(sel.skipped, s)
		} else {
			sel.chosen = append(sel.chosen, s)
		}
	}
	return sel, nil
}

// bulk is the selection with no targets: every running session but skipped
// ones, and pinned and interactive ones unless asked.
func (app *App) bulk(running []claude.Session, skip map[string]bool, withInteractive, withPinned bool) (selection, error) {
	var sel selection
	pins := map[string]bool{}
	if !withPinned {
		var err error
		if pins, err = app.protectedPins(running); err != nil {
			return sel, err
		}
	}
	for _, s := range running {
		switch {
		case skip[s.SessionID]:
			sel.skipped = append(sel.skipped, s)
		case s.Interactive() && !withInteractive:
			sel.leftAlone = append(sel.leftAlone, s)
		case !s.Interactive() && s.JobID == "":
			sel.noJob = append(sel.noJob, s)
		case s.JobID != "" && pins[s.JobID]:
			sel.pinned = append(sel.pinned, s)
		case len(app.Teams.Teammates(s.SessionID)) > 0:
			sel.leads = append(sel.leads, s)
		default:
			sel.chosen = append(sel.chosen, s)
		}
	}
	return sel, nil
}

// sessionVersion is the Claude Code version a session runs: the one its
// process's executable names, or else the job's cliVersion, which Claude Code
// updates only once a respawned session takes a turn.
func (app *App) sessionVersion(s claude.Session) string {
	if s.PID > 0 && app.RunningVersion != nil {
		if v := app.RunningVersion(s.PID); v != "" {
			return v
		}
	}
	return app.Jobs.Version(s.JobID)
}

// keepOutdated narrows the chosen sessions to those on an older Claude Code
// than the installed one.
func (app *App) keepOutdated(ctx context.Context, sel *selection) error {
	installed, err := app.Agents.Version(ctx)
	if err != nil {
		return err
	}
	// Only background sessions the command would act on are checked, so with
	// none of those there is nothing to warn about.
	checked, known := 0, false
	sel.chosen = slices.DeleteFunc(sel.chosen, func(s claude.Session) bool {
		// Interactive sessions are the user's to restart, so --outdated
		// never picks them.
		if s.Interactive() {
			return true
		}
		checked++
		v := app.sessionVersion(s)
		known = known || v != ""
		return v == "" || !version.Less(v, installed)
	})
	if !known && checked > 0 {
		app.warn("claudes: cannot read the Claude Code version of any session from %s. "+
			"Claude Code may have changed its files there.", app.Jobs.Dir)
	}
	return nil
}

// acquire selects sessions and, with --wait, waits until none is busy. A wait
// that times out is reported and returns errReported. A session waiting on a
// question or running a shell task is not waited for: the first needs a
// person, and the second may never finish. Both are pinged instead.
func (app *App) acquire(ctx context.Context, targets []string, f *lifecycleFlags) (selection, error) {
	deadline := app.Now().Add(f.timeout)
	for {
		sel, err := app.selectSessions(ctx, targets, f.selectOpts())
		if err != nil || !f.wait {
			return sel, err
		}
		var busy []string
		for _, s := range sel.chosen {
			if s.Busy() && !app.isHost(s) {
				busy = append(busy, s.Label())
			}
		}
		if len(busy) == 0 {
			return sel, nil
		}
		names := strings.Join(busy, ", ")
		if !app.Now().Before(deadline) {
			app.warn("claudes: still busy after %s (%s). Nothing was changed.", f.timeout, names)
			return sel, errReported
		}
		app.say("Waiting for busy sessions: %s", names)
		if err := app.Sleep(ctx, pollInterval); err != nil {
			return sel, err
		}
	}
}

// skipSet resolves --skip with the same rules as targets. A token that matches
// nothing, or more than one session, is an error, so a typo cannot leave the
// session it meant to protect unprotected.
func skipSet[T target.Ref](skip []string, candidates []T, id func(T) string) (map[string]bool, error) {
	set := map[string]bool{}
	if len(skip) == 0 {
		return set, nil
	}
	picked, err := target.Resolve(skip, candidates)
	if err != nil {
		return nil, fmt.Errorf("--skip: %w", err)
	}
	for _, c := range picked {
		set[id(c)] = true
	}
	return set, nil
}

// requireRunning refuses a named session that is not running.
func requireRunning(sessions []claude.Session) error {
	for _, s := range sessions {
		if !s.Running() {
			return refused(fmt.Errorf("%s is not running (%s): name a running session, or bring it back with claudes resume",
				s.Label(), dash(s.State)))
		}
	}
	return nil
}

// splitHost separates the session running claudes from the others.
func (app *App) splitHost(sessions []claude.Session) ([]claude.Session, *claude.Session) {
	for i, s := range sessions {
		if app.isHost(s) {
			host := s
			return slices.Delete(slices.Clone(sessions), i, i+1), &host
		}
	}
	return sessions, nil
}
