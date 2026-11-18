package cli

import (
	"errors"
	"fmt"

	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/target"
)

// claudes' exit codes, so a script or an agent can tell what happened without
// reading the text.
const (
	exitFailed      = 1   // anything else that went wrong
	exitNoMatch     = 2   // a target named no session, or several
	exitSomeFailed  = 3   // the command reported sessions it could not handle
	exitRefused     = 4   // a safety or compatibility check stopped it
	exitBusy        = 5   // another claudes was changing sessions
	exitInterrupted = 130 // Ctrl-C, or SIGTERM or SIGHUP on Unix
)

// errReported means the command already told the user what went wrong.
var errReported = errors.New("failures were reported")

// errSomeFailed means the command reported sessions it could not handle.
var errSomeFailed = fmt.Errorf("%w: some sessions failed", errReported)

// errBusy means another claudes holds the run lock.
var errBusy = errors.New("another claudes is changing sessions")

type refusalError struct{ error }

func (r refusalError) Unwrap() error { return r.error }

// refusal marks err as a safety or compatibility check that stopped a command
// before it changed anything.
func refusal(err error) error { return refusalError{err} }

type noMatchError struct{ error }

func (n noMatchError) Unwrap() error { return n.error }

// unmatched marks err as a target that names nothing, or several where one is
// needed, found outside internal/target.
func unmatched(err error) error { return noMatchError{err} }

// exitCode is the exit code for an error the command has not reported.
func exitCode(err error) int {
	var match *target.MatchError
	var missed noMatchError
	var refused refusalError
	var refusedSession *selection.RefusedError
	switch {
	case errors.Is(err, errBusy):
		return exitBusy
	case errors.As(err, &refused), errors.As(err, &refusedSession):
		return exitRefused
	case errors.As(err, &match), errors.As(err, &missed):
		return exitNoMatch
	}
	return exitFailed
}
