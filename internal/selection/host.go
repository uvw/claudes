package selection

import (
	"slices"

	"github.com/uvw/claudes/internal/claude"
)

// Host is the session running claudes. Stopping it ends the command, so it
// goes last and is never pinged.
type Host struct {
	// ID is its session id, or "".
	ID string
	// Job is its job id, from CLAUDE_JOB_DIR: a second way to find a
	// background host when ID is not set.
	Job string
}

// Is reports whether s is the host.
func (h Host) Is(s claude.Session) bool {
	return h.IsID(s.SessionID) || h.Job != "" && s.JobID == h.Job
}

// IsID reports whether sessionID is the host's.
func (h Host) IsID(sessionID string) bool {
	return h.ID != "" && sessionID == h.ID ||
		h.Job != "" && claude.ShortID(sessionID) == h.Job
}

// Last moves the host to the end of sessions.
func (h Host) Last(sessions []claude.Session) []claude.Session {
	slices.SortStableFunc(sessions, func(a, b claude.Session) int {
		return TrueFirst(!h.Is(a), !h.Is(b))
	})
	return sessions
}

// Split separates the host from the other sessions.
func (h Host) Split(sessions []claude.Session) ([]claude.Session, *claude.Session) {
	for i, s := range sessions {
		if h.Is(s) {
			host := s
			return slices.Delete(slices.Clone(sessions), i, i+1), &host
		}
	}
	return sessions, nil
}
