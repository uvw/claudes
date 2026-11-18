package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/teams"
)

// gc protects parked sessions, so claudes resume finds them as they were.
// gc --teams does not: a paused lead's task list is removed after a week.
func TestGCTeamsKeepsAParkedSessionsTaskList(t *testing.T) {
	h := newHarness(t)
	paused := stopped(h.bg("paused", ""))
	h.agents.snapshots = [][]claude.Session{{}}
	h.agents.finished = []claude.Session{paused}
	if err := h.app.Park.Update(func(e []park.Entry) []park.Entry {
		return park.Merge(e, []park.Entry{park.FromSession(paused, false, h.now.Format(time.RFC3339))})
	}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(h.app.Teams.Tasks, teams.Name(paused.SessionID), "1.json")
	writeFile(t, p, `{"id":"1","subject":"half done","status":"in_progress"}`)
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	h.ok(t, "gc", "--teams")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("gc --teams removed the task list of a parked session: %v\n%s", err, h.out)
	}
}

// A pin protects a session's team and task list once the session has stopped,
// whether or not claude agents still lists it, unless --pinned.
func TestGCTeamsKeepsAStoppedPinnedSessionsTeam(t *testing.T) {
	h := newHarness(t)
	listed := stopped(h.bg("listed", ""))
	unlisted := stopped(h.bg("unlisted", ""))
	unlisted.JobID = unlisted.ShortID()
	h.agents.snapshots = [][]claude.Session{{}}
	h.agents.finished = []claude.Session{listed}
	h.pin(t, listed, unlisted)
	old := h.now.Add(-30 * 24 * time.Hour)
	var paths []string
	for _, s := range []claude.Session{listed, unlisted} {
		paths = append(paths, filepath.Join(h.app.Teams.Teams, teams.Name(s.SessionID), "config.json"),
			filepath.Join(h.app.Teams.Tasks, teams.Name(s.SessionID), "1.json"))
	}
	for _, p := range paths {
		writeFile(t, p, "{}")
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	h.ok(t, "gc", "--teams")
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("gc --teams removed a pinned session's %s: %v\n%s", p, err, h.out)
		}
	}
	h.ok(t, "gc", "--teams", "--pinned")
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("gc --teams --pinned kept %s", p)
		}
	}
}

func TestGCLeavesParkedAndRecentlyActiveSessionsAlone(t *testing.T) {
	h := newHarness(t)
	old, parkedOld, recent := stopped(h.bg("old", "")), stopped(h.bg("parked-old", "")), stopped(h.bg("recent", ""))
	for _, s := range []*claude.Session{&old, &parkedOld, &recent} {
		s.StartedAt = h.now.Add(-30 * 24 * time.Hour).UnixMilli()
	}
	writeState(t, h, recent.JobID, "2.1.325")
	h.agents.finished = []claude.Session{old, parkedOld, recent}
	if err := h.app.parkOne(parkedOld, false); err != nil {
		t.Fatal(err)
	}

	h.ok(t, "gc")

	assertCalls(t, h, "rm "+old.JobID)
}

func TestOlderThanShowsItsDefaultInDays(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "gc", "--help")
	assertContains(t, h.out.String(), "--older-than duration")
	assertContains(t, h.out.String(), "(default 7d)")
	if s := (&days{d: 90 * time.Minute}).String(); s != "1h30m0s" {
		t.Fatalf("90m: %q", s)
	}
}

// --confirm asks before gc and gc --teams change anything, as it does on
// restart, and an answer of no changes nothing.
func TestGCConfirmAsksFirst(t *testing.T) {
	h := newHarness(t)
	old := stopped(h.bg("old", ""))
	old.StartedAt = h.now.Add(-10 * 24 * time.Hour).UnixMilli()
	h.agents.snapshots = [][]claude.Session{{}}
	h.agents.finished = []claude.Session{old}
	p := filepath.Join(h.app.Teams.Tasks, teams.Name(old.SessionID), "1.json")
	writeFile(t, p, "{}")
	h.aged(t, filepath.Dir(p), 30*24*time.Hour)

	h.app.In = strings.NewReader("n\n")
	h.fails(t, "gc", "--confirm")
	h.app.In = strings.NewReader("n\n")
	h.fails(t, "gc", "--teams", "--confirm")
	assertCalls(t, h)
	if !exists(p) {
		t.Fatal("gc --teams --confirm removed a task list it was told not to")
	}

	h.app.In = strings.NewReader("y\n")
	h.ok(t, "gc", "--confirm")
	assertCalls(t, h, "rm "+old.JobID)
	assertContains(t, h.out.String(), "Remove these sessions from the list? [y/N]")
}

// gc --teams moves what it removes to the trash, as session gc does, and
// --purge deletes it for good.
func TestGCTeamsTrashesUnlessPurged(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{}}
	tasks := func(name string) string {
		p := filepath.Join(h.app.Teams.Tasks, teams.Name(sessionID(name)), "1.json")
		writeFile(t, p, "{}")
		h.aged(t, filepath.Dir(p), 30*24*time.Hour)
		return filepath.Dir(p)
	}
	first := tasks("first")
	h.ok(t, "gc", "--teams")
	if len(h.trashed) != 1 || h.trashed[0] != first {
		t.Fatalf("trashed %q, want %s", h.trashed, first)
	}
	assertContains(t, h.out.String(), "trashed")

	second := tasks("second")
	h.trashed = nil
	h.ok(t, "gc", "--teams", "--purge")
	if len(h.trashed) != 0 || exists(second) {
		t.Fatalf("--purge trashed %q, or kept %s", h.trashed, second)
	}
	h.fails(t, "gc", "--purge")
	assertContains(t, h.err.String(), "--purge goes with --teams")
}
