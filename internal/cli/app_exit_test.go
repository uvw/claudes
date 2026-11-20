package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/target"
)

func TestExitCodeFollowsTheWrappedError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{fmt.Errorf("%w, so this one changed nothing", errBusy), exitBusy},
		{refusal(errors.New("a live session")), exitRefused},
		{fmt.Errorf("gc: %w", refusal(errors.New("a live session"))), exitRefused},
		{&selection.RefusedError{Err: errors.New("a live session")}, exitRefused},
		{&target.MatchError{}, exitNoMatch},
		{unmatched(errors.New("2 sessions match")), exitNoMatch},
		{errors.New("disk full"), exitFailed},
	} {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestMarkedErrorsKeepTheirText(t *testing.T) {
	cause := errors.New("a live session")
	for _, err := range []error{refusal(cause), unmatched(cause)} {
		if err.Error() != cause.Error() || !errors.Is(err, cause) {
			t.Errorf("%v does not wrap %v", err, cause)
		}
	}
	if !errors.Is(errSomeFailed, errReported) {
		t.Error("errSomeFailed is not errReported")
	}
}
