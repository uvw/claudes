package cli

import (
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestANameOfAStoppedSessionNeverPicksARunningOne(t *testing.T) {
	h := newHarness(t)
	gw := h.bg("api-gw", "idle")
	h.agents.snapshots = [][]claude.Session{{gw}}
	h.agents.finished = []claude.Session{stopped(h.bg("api", ""))}

	for _, cmd := range []string{"restart", "stop", "pause"} {
		h.err.Reset()
		h.fails(t, cmd, "api")
		assertContains(t, h.err.String(), "api is not running")
	}
	assertCalls(t, h)
}

func TestPinnedTakesPinnedSessionsInBulk(t *testing.T) {
	h := newHarness(t)
	a, p := h.bg("alpha", "idle"), h.bg("pinned", "idle")
	h.agents.snapshots = [][]claude.Session{{a, p}}
	h.pin(t, p)

	h.ok(t, "restart")
	assertContains(t, h.out.String(), "Add --pinned to include them. Named sessions replace the bulk selection, so name every session you want.")

	h.out.Reset()
	h.agents.calls = nil
	h.ok(t, "restart", "--pinned")
	assertCalls(t, h, "respawn "+a.JobID, "respawn "+p.JobID)
	if strings.Contains(h.out.String(), "Pinned, not") {
		t.Fatalf("--pinned still left a pinned session out:\n%s", h.out)
	}

	h.agents.calls = nil
	h.ok(t, "pause", "--pinned")
	assertCalls(t, h, "stop "+a.JobID, "stop "+p.JobID)
	if entries, err := h.app.Park.Load(); err != nil || len(entries) != 2 {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
}
