package cli

import (
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestSkipThatMatchesNothingStopsTheCommand(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("db-migration", "idle"), h.bg("other", "idle")}}

	h.fails(t, "restart", "--skip", "db-migrate")

	assertCalls(t, h)
	assertContains(t, h.err.String(), `--skip: no session matches "db-migrate"`)
}

func TestSkipMatchesLikeATargetAndIsListed(t *testing.T) {
	h := newHarness(t)
	db, other := h.bg("db-migration", "idle"), h.bg("other", "idle")
	h.agents.snapshots = [][]claude.Session{{db, other}}

	h.ok(t, "restart", "--skip", "DB")

	assertCalls(t, h, "respawn "+other.JobID)
	assertContains(t, h.out.String(), "Skipped, not restarted: db-migration")
}

func TestBulkCommandsLeaveABackgroundSessionWithoutAJobIDAlone(t *testing.T) {
	h := newHarness(t)
	a, odd := h.bg("alpha", "idle"), h.bg("odd", "idle")
	odd.JobID = ""
	h.agents.snapshots = [][]claude.Session{{a, odd}}

	h.ok(t, "restart")

	assertCalls(t, h, "respawn "+a.JobID)
	assertContains(t, h.out.String(), "gave no job id: odd")
}

func TestOutdatedWarnsWhenNoVersionCanBeRead(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}

	h.ok(t, "restart", "--outdated")

	assertCalls(t, h)
	assertContains(t, h.err.String(), "cannot read the Claude Code version of any session")
}

func TestOutdatedWithEverySessionPinnedDoesNotWarn(t *testing.T) {
	h := newHarness(t)
	p := h.bg("pinned", "idle")
	h.agents.snapshots = [][]claude.Session{{p}}
	h.pin(t, p)

	h.ok(t, "restart", "--outdated")

	assertCalls(t, h)
	if strings.Contains(h.err.String(), "cannot read the Claude Code version") {
		t.Fatalf("warned about versions with no session to check:\n%s", h.err)
	}
}
