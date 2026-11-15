// Package claude wraps the parts of the claude command line that claudes uses.
package claude

import (
	"regexp"
	"time"

	"github.com/uvw/claudes/internal/shell"
)

// The kinds of session claude agents --json lists.
const (
	KindBackground  = "background"
	KindInteractive = "interactive"
)

// Session is one entry of `claude agents --json`. claudes prints it with the
// same camelCase names, and with every key present, so a script can rely on
// each field.
type Session struct {
	// JobID is the short id that `claude stop`, `respawn`, `rm`, `logs`, and
	// `attach` take. Only background sessions have one.
	JobID string `json:"id"`
	// SessionID is the full id that `claude --resume` takes.
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Cwd       string `json:"cwd"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	// StartedAt is in milliseconds since the Unix epoch.
	StartedAt int64 `json:"startedAt"`
}

// Started is when the session started, or the zero time when unknown.
func (s Session) Started() time.Time {
	if s.StartedAt == 0 {
		return time.Time{}
	}
	return time.UnixMilli(s.StartedAt)
}

// Running reports whether the session has a live process.
func (s Session) Running() bool { return s.PID > 0 }

// Busy reports whether the session is working on a turn right now. The
// status lags the state: a session that just took a prompt reads state
// working with status idle for a moment, so either counts.
func (s Session) Busy() bool {
	return s.Status == "busy" || s.State == "working" && (s.Status == "idle" || s.Status == "")
}

// MidTurn reports whether stopping the session would cut work short: it is
// busy, waiting on a question or permission prompt, or idle with a background
// shell task such as a build still running. Such a session comes back idle
// after a restart unless it is pinged.
func (s Session) MidTurn() bool {
	switch s.Status {
	case "waiting", "shell":
		return true
	}
	return s.Busy()
}

// Activity is the status to show: busy for a session in a turn whose status
// has not caught up yet.
func (s Session) Activity() string {
	if s.Busy() {
		return "busy"
	}
	return s.Status
}

// Interactive reports whether the session is a TUI in a terminal.
func (s Session) Interactive() bool { return s.Kind == KindInteractive }

// ShortID is the first eight characters of the session id.
func (s Session) ShortID() string { return ShortID(s.SessionID) }

// ID is the id to show: the job id when there is one.
func (s Session) ID() string {
	if s.JobID != "" {
		return s.JobID
	}
	return s.ShortID()
}

// Label is the name to show: the session name, or its id when it has none.
func (s Session) Label() string {
	if s.Name != "" {
		return shell.Printable(s.Name)
	}
	return s.ID()
}

// Aliases are the exact strings that select this session.
func (s Session) Aliases() []string {
	return NonEmpty(s.Name, s.JobID, s.SessionID, s.ShortID())
}

// ManualCommand is the shell command that brings the session back by hand.
func (s Session) ManualCommand() string {
	return shell.ResumeCommand(s.SessionID, s.Cwd, s.Interactive())
}

var sessionID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidSessionID reports whether id is a session id: a UUID. claudes prints
// session ids in commands, so anything else is refused.
func ValidSessionID(id string) bool { return sessionID.MatchString(id) }

// ShortID is the first eight characters of a session id, as agent view shows
// it.
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// NonEmpty is values without the empty ones, for building a target's aliases.
func NonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
