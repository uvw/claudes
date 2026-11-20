package cli

import (
	"context"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/target"
)

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
func (app *App) selectSessions(ctx context.Context, targets []string, opts selectOpts) (selection.Selection, error) {
	// Targets are matched against finished sessions too, so a name that
	// belongs to a stopped session never falls through to a running one whose
	// name merely starts with it.
	sessions, err := app.listed(ctx, len(targets) > 0)
	if err != nil {
		return selection.Selection{}, err
	}
	running := slices.DeleteFunc(slices.Clone(sessions), func(s claude.Session) bool { return !s.Running() })
	skip, err := selection.Skip(opts.skip, running, func(s claude.Session) string { return s.SessionID })
	if err != nil {
		return selection.Selection{}, err
	}
	var sel selection.Selection
	if len(targets) > 0 {
		sel, err = selection.Named(targets, sessions, skip, opts.withInteractive)
	} else {
		sel, err = app.bulk(running, skip, opts.withInteractive, opts.withPinned)
	}
	if err != nil {
		return selection.Selection{}, err
	}
	if opts.outdated {
		sel.Outdated = true
		if err := app.keepOutdated(ctx, &sel); err != nil {
			return selection.Selection{}, err
		}
	}
	sel.Chosen = app.host().Last(sel.Chosen)
	return sel, nil
}

// bulk is the selection with no targets: every running session but skipped
// ones, and pinned and interactive ones unless asked.
func (app *App) bulk(running []claude.Session, skip map[string]bool, withInteractive, withPinned bool) (selection.Selection, error) {
	pins := map[string]bool{}
	if !withPinned {
		var err error
		if pins, err = app.protectedPins(running); err != nil {
			return selection.Selection{}, err
		}
	}
	return selection.Bulk(running, skip, pins, withInteractive, app.Teams.Teammates), nil
}

// keepOutdated narrows the chosen sessions to those on an older Claude Code
// than the installed one.
func (app *App) keepOutdated(ctx context.Context, sel *selection.Selection) error {
	installed, err := app.Agents.Version(ctx)
	if err != nil {
		return err
	}
	if selection.KeepOutdated(sel, installed, app.sessionVersion) {
		app.warn("claudes: cannot read the Claude Code version of any session from %s. "+
			"Claude Code may have changed its files there.", app.Jobs.Dir)
	}
	return nil
}

// acquire selects sessions and, with --wait, waits until none is busy. A wait
// that times out is reported and returns errReported. A session waiting on a
// question or running a shell task is not waited for: the first needs a
// person, and the second may never finish. Both are pinged instead.
func (app *App) acquire(ctx context.Context, targets []string, f *lifecycleFlags) (selection.Selection, error) {
	deadline := app.Now().Add(f.timeout)
	for {
		sel, err := app.selectSessions(ctx, targets, f.selectOpts())
		if err != nil || !f.wait {
			return sel, err
		}
		var busy []string
		for _, s := range sel.Chosen {
			if s.Busy() && !app.host().Is(s) {
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

// listMatch keeps the rows any of targets match. A target that matches
// nothing is noted on stderr and leaves the rest as they are, so a listing
// shows what it found, as kubectl get does for a selector; where and the
// commands that act still fail on it.
func listMatch[T target.Ref](app *App, targets []string, rows []T) ([]T, error) {
	out, missed, err := target.Filter(targets, rows)
	for _, token := range missed {
		app.warn("claudes: no session matches %q", shell.Printable(token))
	}
	return out, err
}
