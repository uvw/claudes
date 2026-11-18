package cli

import (
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
