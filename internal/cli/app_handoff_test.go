package cli

import (
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestHandOffRefusesAnInteractiveSession(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.tty("terminal", "idle")}}

	h.fails(t, "logs", "terminal")
	if len(h.execs) != 0 {
		t.Fatalf("execs %q", h.execs)
	}
	assertContains(t, h.err.String(), "claude logs works on background sessions")
}

func TestSessionResumeAndMvRefuseWhenTheyCannotTellWhatRuns(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	h.transcript(t, "plan", h.folder("from"), time.Hour)
	h.agents.failures = map[string]string{"list": "claude is gone"}

	h.fails(t, "session", "resume", "plan")
	assertContains(t, h.err.String(), "cannot tell whether")
	h.err.Reset()
	h.fails(t, "session", "mv", "plan", to)
	assertContains(t, h.err.String(), "pass --force")
	h.ok(t, "session", "mv", "--force", "plan", to)
	if len(h.execs) != 0 {
		t.Fatalf("execs %q", h.execs)
	}
}
