package cli

import (
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestResumeSkipsAndTargetsAParkedSessionByJobID(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("bravo", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}
	h.ok(t, "pause")
	h.world([]claude.Session{})

	h.ok(t, "resume", "--skip", b.JobID)
	assertCalls(t, h, "resume "+a.SessionID+" in alpha prompt=")

	h.world([]claude.Session{})
	h.ok(t, "resume", b.JobID)
	assertCalls(t, h, "resume "+b.SessionID+" in bravo prompt=")
}

func TestForgetDryRunDropsNothing(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}
	h.ok(t, "pause")
	h.world([]claude.Session{})

	h.ok(t, "resume", "--forget", "--dry-run", "alpha")

	assertContains(t, h.out.String(), "Would drop from the park list: alpha")
	if entries, _ := h.app.Park.Load(); len(entries) != 1 {
		t.Fatalf("entries %+v", entries)
	}
}
