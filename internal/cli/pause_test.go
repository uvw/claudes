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

func TestPauseSavesTheListBeforeStopping(t *testing.T) {
	h := newHarness(t)
	a, busy, p := h.bg("alpha", "idle"), h.bg("busy", "busy"), h.bg("pinned", "idle")
	h.agents.snapshots = [][]claude.Session{{a, busy, p}}
	h.pin(t, p)
	h.agents.beforeStop = func(string) {
		if entries, _ := h.app.Park.Load(); len(entries) != 2 {
			t.Errorf("stopped with %d session(s) parked", len(entries))
		}
	}

	h.ok(t, "pause")

	assertCalls(t, h, "stop "+a.JobID, "stop "+busy.JobID)
	entries, err := h.app.Park.Load()
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries %v, err %v", entries, err)
	}
	if entries[0].WasBusy || !entries[1].WasBusy {
		t.Fatalf("busy flags wrong: %+v", entries)
	}
	assertContains(t, h.out.String(), "Bring them back with: claudes resume")
}

func TestPauseDropsASessionThatWouldNotStop(t *testing.T) {
	h := newHarness(t)
	bad, good := h.bg("bad", "idle"), h.bg("good", "idle")
	h.agents.snapshots = [][]claude.Session{{bad, good}}
	h.agents.failures = map[string]string{"stop " + bad.JobID: "boom"}

	h.fails(t, "pause")

	entries, _ := h.app.Park.Load()
	if len(entries) != 1 || entries[0].Name != "good" {
		t.Fatalf("entries %+v", entries)
	}
}

func TestAWorkingSessionWithALaggingStatusIsPausedAsBusy(t *testing.T) {
	h := newHarness(t)
	s := h.bg("counting", "idle")
	s.State = "working"
	h.agents.snapshots = [][]claude.Session{{s}}

	h.ok(t, "pause", "counting")
	assertContains(t, h.out.String(), "busy")
	entries, err := h.app.Park.Load()
	if err != nil || len(entries) != 1 || !entries[0].WasBusy {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
	h.agents.calls = nil
	h.ok(t, "resume", "--ping")
	assertCalls(t, h, "resume "+s.SessionID+" in counting prompt="+defaultPing)
}
