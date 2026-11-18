package exit

import (
	"errors"
	"fmt"
	"testing"

	"github.com/uvw/claudes/internal/target"
)

func TestCodeFollowsTheWrappedError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{fmt.Errorf("%w, so this one changed nothing", ErrBusy), Busy},
		{Refusal(errors.New("a live session")), Refused},
		{fmt.Errorf("gc: %w", Refusal(errors.New("a live session"))), Refused},
		{&target.MatchError{}, NoMatch},
		{Unmatched(errors.New("2 sessions match")), NoMatch},
		{errors.New("disk full"), Failed},
	} {
		if got := Code(tc.err); got != tc.want {
			t.Errorf("Code(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestMarkedErrorsKeepTheirText(t *testing.T) {
	cause := errors.New("a live session")
	for _, err := range []error{Refusal(cause), Unmatched(cause)} {
		if err.Error() != cause.Error() || !errors.Is(err, cause) {
			t.Errorf("%v does not wrap %v", err, cause)
		}
	}
	if !errors.Is(ErrSomeFailed, ErrReported) {
		t.Error("ErrSomeFailed is not ErrReported")
	}
}
