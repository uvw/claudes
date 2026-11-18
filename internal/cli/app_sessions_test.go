package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestBulkCommandsLeaveALeadWithTeammatesAlone(t *testing.T) {
	h := newHarness(t)
	lead, other := h.bg("lead", "idle"), h.bg("other", "idle")
	h.agents.snapshots = [][]claude.Session{{lead, other}}
	h.team(t, lead, "writer", "tester")

	h.ok(t, "restart", "--dry-run")
	out := h.out.String()
	assertContains(t, out, "Leading an agent team, not restarted: lead")
	if strings.Contains(out, "  "+lead.JobID+"  ") {
		t.Fatalf("the lead is in the plan:\n%s", out)
	}
	assertContains(t, out, "To include it anyway, name it with every other session you want.")
	h.out.Reset()
	h.ok(t, "pause", "--dry-run")
	assertContains(t, h.out.String(), "Leading an agent team, not paused: lead")

	// Naming the lead takes it alone, not the bulk selection plus the lead.
	h.agents.calls = nil
	h.ok(t, "restart", "lead")
	assertCalls(t, h, "respawn "+lead.JobID)
}

func TestANamedLeadIsRestartedAndPingedWithAWarning(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead, "writer")

	h.ok(t, "restart", "--ping", "lead")
	assertContains(t, h.out.String(), "leads 1 teammate(s): writer, which stop with it")
	assertCalls(t, h, "stop "+lead.JobID, "resume "+lead.SessionID+" in lead prompt="+defaultPing)

	h.err.Reset()
	h.ok(t, "stop", "lead")
	assertContains(t, h.err.String(), "lead leads 1 teammate(s): writer")
}

func TestALeadWhoseTeammatesShutDownIsAnOrdinarySession(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead)

	h.ok(t, "restart", "--dry-run")
	assertContains(t, h.out.String(), "Restarting 1 session(s)")
}

func TestSessionShowAndWherePickTheNewestOfAName(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("before"), 48*time.Hour)
	h.transcript(t, "plan", h.folder("after"), time.Hour)
	h.transcript(t, "planner", h.folder("other"), 2*time.Hour)

	if code := h.run("session", "where", "--one", "plan"); code != 0 || strings.TrimSpace(h.out.String()) != h.folder("after") {
		t.Fatalf("exit %d, %q", code, h.out)
	}
	h.out.Reset()
	h.ok(t, "session", "show", "plan")
	assertContains(t, h.out.String(), h.folder("after"))
	assertContains(t, h.out.String(), "carry on …")
}

func TestAnUnnamedLiveSessionKeepsItsTranscriptName(t *testing.T) {
	h := newHarness(t)
	live := h.bg("plan", "idle")
	h.transcriptAs(t, live.SessionID, "plan", h.folder("plan"), time.Minute)
	live.Name = live.JobID
	h.agents.snapshots = [][]claude.Session{{live}}

	h.ok(t, "session", "show", "plan")
	assertContains(t, h.out.String(), "plan")
}
