package cli

import (
	"errors"
	"os"
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

func TestResumeBringsParkedSessionsBackAndClearsTheFile(t *testing.T) {
	h := newHarness(t)
	a, busy := h.bg("alpha", "idle"), h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{a, busy}, {}}
	h.ok(t, "pause")
	h.agents.calls = nil

	h.ok(t, "resume", "--ping")

	assertCalls(t, h,
		"resume "+a.SessionID+" in alpha prompt=",
		"resume "+busy.SessionID+" in busy prompt="+defaultPing,
	)
	if _, err := os.Stat(h.app.Park.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("park file still there: %v", err)
	}
}

func TestResumeKeepsFailuresAndSkipsRunningSessions(t *testing.T) {
	h := newHarness(t)
	a, b, c := h.bg("alpha", "idle"), h.bg("bravo", "idle"), h.bg("charlie", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b, c}}
	h.ok(t, "pause")
	h.world([]claude.Session{c})
	h.agents.failures = map[string]string{"resume " + b.SessionID + " in bravo prompt=": "directory is gone"}

	h.fails(t, "resume")

	entries, _ := h.app.Park.Load()
	if len(entries) != 1 || entries[0].Name != "bravo" {
		t.Fatalf("entries %+v", entries)
	}
	assertContains(t, h.out.String(), "Already running, dropped from the park list: charlie")
}

func TestResumeSkipKeepsTheEntryParked(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("bravo", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}, {}}
	h.ok(t, "pause")
	h.agents.calls = nil

	h.ok(t, "resume", "--skip", "bravo")

	assertCalls(t, h, "resume "+a.SessionID+" in alpha prompt=")
	entries, _ := h.app.Park.Load()
	if len(entries) != 1 || entries[0].Name != "bravo" {
		t.Fatalf("entries %+v", entries)
	}
}

func TestResumeRecoversAStoppedSessionByName(t *testing.T) {
	h := newHarness(t)
	db := stopped(h.bg("db-upgrade-plan", ""))
	h.agents.snapshots = [][]claude.Session{{}}
	h.agents.finished = []claude.Session{db}

	h.ok(t, "resume", "db", "--ping")

	assertCalls(t, h, "resume "+db.SessionID+" in db-upgrade-plan prompt="+defaultPing)
}

func TestNothingParked(t *testing.T) {
	h := newHarness(t, nil)
	h.ok(t, "resume")
	assertContains(t, h.out.String(), "Nothing is parked.")
}

func TestForgetOfAnUnparkedNameKeepsTheFile(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}
	h.ok(t, "pause", "alpha")
	before, _ := os.ReadFile(h.app.Park.Path)

	h.fails(t, "resume", "--forget", "nobody")
	h.fails(t, "resume", "--forget")
	if after, _ := os.ReadFile(h.app.Park.Path); string(after) != string(before) {
		t.Fatalf("park file changed:\n%s", after)
	}
	assertContains(t, h.err.String(), "--forget needs the sessions to drop")
}
