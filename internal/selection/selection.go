// Package selection decides which sessions a bulk command acts on: the ones
// named, or every running one but those it leaves out and why, and the session
// running claudes, which goes last.
package selection

import (
	"fmt"
	"slices"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/version"
)

// Selection is what a restart or pause acts on.
type Selection struct {
	Chosen []claude.Session
	// LeftAlone are running interactive sessions nobody asked to touch.
	LeftAlone []claude.Session
	// Pinned are running sessions skipped because they are pinned.
	Pinned []claude.Session
	// Skipped are the sessions --skip named.
	Skipped []claude.Session
	// NoJob are background sessions claude agents gave no job id, which
	// claude cannot stop or respawn.
	NoJob []claude.Session
	// Leads are sessions skipped because their agent team has teammates at
	// work, which would stop with them and could not be resumed.
	Leads []claude.Session
	// Outdated is set by --outdated, which never picks interactive sessions.
	Outdated bool
}

// Named is the selection for explicit targets: exactly those sessions.
func Named(targets []string, sessions []claude.Session, skip map[string]bool, withInteractive bool) (Selection, error) {
	var sel Selection
	picked, err := target.Resolve(targets, sessions)
	if err != nil {
		return sel, err
	}
	if err := RequireRunning(picked); err != nil {
		return sel, err
	}
	for _, s := range picked {
		if s.Interactive() && !withInteractive {
			return sel, refused(fmt.Errorf("%s is an interactive session: add --interactive or --interactive-bg", s.Label()))
		}
		if skip[s.SessionID] {
			sel.Skipped = append(sel.Skipped, s)
		} else {
			sel.Chosen = append(sel.Chosen, s)
		}
	}
	return sel, nil
}

// Bulk is the selection with no targets: every running session but skipped
// ones, interactive ones unless withInteractive, ones without a job id, the
// pinned ones in pins, and leads whose teammates are at work.
func Bulk(running []claude.Session, skip, pins map[string]bool, withInteractive bool, teammates func(sessionID string) []string) Selection {
	var sel Selection
	for _, s := range running {
		switch {
		case skip[s.SessionID]:
			sel.Skipped = append(sel.Skipped, s)
		case s.Interactive() && !withInteractive:
			sel.LeftAlone = append(sel.LeftAlone, s)
		case !s.Interactive() && s.JobID == "":
			sel.NoJob = append(sel.NoJob, s)
		case s.JobID != "" && pins[s.JobID]:
			sel.Pinned = append(sel.Pinned, s)
		case len(teammates(s.SessionID)) > 0:
			sel.Leads = append(sel.Leads, s)
		default:
			sel.Chosen = append(sel.Chosen, s)
		}
	}
	return sel
}

// KeepOutdated narrows the chosen sessions to background ones on an older
// Claude Code than installed. versionOf is the version a session runs, or ""
// when unknown. It reports whether it checked sessions but could read the
// version of none.
func KeepOutdated(sel *Selection, installed string, versionOf func(claude.Session) string) (unknown bool) {
	checked, known := 0, false
	sel.Chosen = slices.DeleteFunc(sel.Chosen, func(s claude.Session) bool {
		// Interactive sessions are the user's to restart, so --outdated
		// never picks them.
		if s.Interactive() {
			return true
		}
		checked++
		v := versionOf(s)
		known = known || v != ""
		return v == "" || !version.Less(v, installed)
	})
	// Only background sessions the command would act on are checked, so with
	// none of those there is nothing to warn about.
	return !known && checked > 0
}

// Skip resolves --skip with the same rules as targets. A token that matches
// nothing, or more than one session, is an error, so a typo cannot leave the
// session it meant to protect unprotected.
func Skip[T target.Ref](skip []string, candidates []T, id func(T) string) (map[string]bool, error) {
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

// RefusedError is a session a command must not act on, found before it
// changed anything.
type RefusedError struct{ Err error }

func (e *RefusedError) Error() string { return e.Err.Error() }

func (e *RefusedError) Unwrap() error { return e.Err }

func refused(err error) error { return &RefusedError{Err: err} }

// RequireRunning refuses a named session that is not running.
func RequireRunning(sessions []claude.Session) error {
	for _, s := range sessions {
		if !s.Running() {
			return refused(fmt.Errorf("%s is not running (%s): name a running session, or bring it back with claudes resume",
				s.Label(), render.Dash(s.State)))
		}
	}
	return nil
}

// RunningIn reports whether a session that match picks is running.
func RunningIn(sessions []claude.Session, match func(claude.Session) bool) bool {
	return slices.ContainsFunc(sessions, func(s claude.Session) bool { return match(s) && s.Running() })
}

var finishedStates = []string{"done", "stopped", "failed"}

// IsFinished reports whether gc may remove s: a background session that is
// done, stopped, or failed, with no process.
func IsFinished(s claude.Session) bool {
	return !s.Running() && s.JobID != "" && slices.Contains(finishedStates, s.State)
}

// RequireFinished refuses a named session gc may not remove.
func RequireFinished(sessions []claude.Session) error {
	for _, s := range sessions {
		switch {
		case s.Interactive():
			return refused(fmt.Errorf("%s is an interactive session: gc removes only finished background sessions", s.Label()))
		case s.Running():
			return refused(fmt.Errorf("%s is still running: gc removes only finished background sessions. Stop it first", s.Label()))
		case !IsFinished(s):
			return refused(fmt.Errorf("%s is not finished (%s): gc removes only finished background sessions", s.Label(), render.Dash(s.State)))
		}
	}
	return nil
}

// TrueFirst orders a before b when a is true and b is not, for sorting.
func TrueFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}
