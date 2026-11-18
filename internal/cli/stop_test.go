package cli

import (
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestStopParksAnInteractiveSessionAndPrintsHowToResumeIt(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{term}}

	h.ok(t, "stop", "terminal")

	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
	if entries, _ := h.app.Park.Load(); len(entries) != 1 || entries[0].SessionID != term.SessionID {
		t.Fatalf("parked %+v", entries)
	}
}
