package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

// A PID reused by another process after claude stop makes the wait fail,
// and the session stays stopped though it could have been resumed.
func TestAReusedPIDDoesNotBlockThePing(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "busy")
	h.agents.snapshots = [][]claude.Session{{s}}
	newer := start.Add(time.Minute)
	h.app.Alive = func(pid int, started time.Time) bool { return pid == s.PID && started.Equal(newer) }
	code := h.run("ping", "api", "carry on")
	if code != 0 {
		t.Fatalf("exit %d: the session was stopped and not resumed:\n%s\n%s", code, h.out, h.err)
	}
}

type slowStart struct{ *fakeAgents }

func (s slowStart) Resume(_ context.Context, sessionID, _, _ string, _ claude.NewJob) (string, error) {
	return "", s.do("resume", sessionID)
}

// claude --bg --resume returned, so the session is on its way. Telling the
// user it was not resumed, and to run claude --resume, starts a copy if it
// comes up late.
func TestASlowStartIsNotReportedAsNotResumed(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "busy")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.app.Agents = slowStart{h.agents}
	h.run("ping", "api", "carry on")
	msg := h.err.String()
	if strings.Contains(msg, "not resumed") || strings.Contains(msg, "claude --resume") {
		t.Fatalf("a resume that was issued is reported as not done:\n%s", msg)
	}
}
