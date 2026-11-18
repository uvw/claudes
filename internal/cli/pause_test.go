package cli

import (
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

// A lead whose teammates are at work is mid-turn though its own status is
// idle, so pausing it marks it for resume --ping, as restart --ping would.
func TestAPausedLeadIsPingedOnResume(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{lead}, {}}
	h.team(t, lead, "writer")

	h.ok(t, "pause", "lead")
	entries, err := h.app.Park.Load()
	if err != nil || len(entries) != 1 || !entries[0].WasBusy {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
	h.agents.calls = nil
	h.ok(t, "resume", "--ping")
	assertCalls(t, h, "resume "+lead.SessionID+" in lead prompt="+defaultPing)
}
