package cli

import (
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestTeamAnswersToTAndTeams(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "busy")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead, "writer", "tester")

	for _, alias := range []string{"t", "teams"} {
		h.out.Reset()
		h.ok(t, alias, "ls")
		assertContains(t, h.out.String(), "writer, tester")
	}
	if len(h.execs) != 0 {
		t.Fatalf("an alias passed through to claude agents: %q", h.execs)
	}
}
