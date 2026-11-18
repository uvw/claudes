// Package exit holds claudes' exit codes and the errors that choose them, so
// a script or an agent can tell what happened without reading the text.
package exit

import (
	"errors"
	"fmt"

	"github.com/uvw/claudes/internal/target"
)

// ErrReported means the command already told the user what went wrong.
var ErrReported = errors.New("failures were reported")

// ErrSomeFailed means the command reported sessions it could not handle.
var ErrSomeFailed = fmt.Errorf("%w: some sessions failed", ErrReported)

const (
	Failed      = 1   // anything else that went wrong
	NoMatch     = 2   // a target named no session, or several
	SomeFailed  = 3   // the command reported sessions it could not handle
	Refused     = 4   // a safety or compatibility check stopped it
	Busy        = 5   // another claudes was changing sessions
	Interrupted = 130 // Ctrl-C, or SIGTERM or SIGHUP on Unix
)

// ErrBusy means another claudes holds the run lock.
var ErrBusy = errors.New("another claudes is changing sessions")

type refusal struct{ error }

func (r refusal) Unwrap() error { return r.error }

// Refusal marks err as a safety or compatibility check that stopped a command
// before it changed anything.
func Refusal(err error) error { return refusal{err} }

type noMatch struct{ error }

func (n noMatch) Unwrap() error { return n.error }

// Unmatched marks err as a target that names nothing, or several where one is
// needed, found outside internal/target.
func Unmatched(err error) error { return noMatch{err} }

// Code is the exit code for an error the command has not reported.
func Code(err error) int {
	var match *target.MatchError
	var missed noMatch
	var refusal refusal
	switch {
	case errors.Is(err, ErrBusy):
		return Busy
	case errors.As(err, &refusal):
		return Refused
	case errors.As(err, &match), errors.As(err, &missed):
		return NoMatch
	}
	return Failed
}
