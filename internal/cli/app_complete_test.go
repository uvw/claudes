package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

// complete runs cobra's completion request and returns the offered names.
func (h *harness) complete(t *testing.T, args ...string) []string {
	t.Helper()
	h.out.Reset()
	h.ok(t, append([]string{"__complete"}, args...)...)
	var names []string
	for _, line := range strings.Split(h.out.String(), "\n") {
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		name, _, _ := strings.Cut(line, "\t")
		names = append(names, name)
	}
	return names
}

func assertOffers(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("offers\n got: %q\nwant: %q", got, want)
	}
}

func TestEachCommandOffersTheSessionsItCanActOn(t *testing.T) {
	h := newHarness(t)
	idle, pinned, term := h.bg("idle", "idle"), h.bg("pinned", "idle"), h.tty("terminal", "idle")
	done := stopped(h.bg("done", ""))
	h.agents.snapshots = [][]claude.Session{{idle, pinned, term}}
	h.agents.finished = []claude.Session{done}
	h.pin(t, pinned)

	assertOffers(t, h.complete(t, "stop", ""), "idle", "pinned", "terminal")
	assertOffers(t, h.complete(t, "restart", ""), "idle", "pinned", "terminal")
	assertOffers(t, h.complete(t, "ping", ""), "idle", "pinned", "done")
	assertOffers(t, h.complete(t, "logs", ""), "idle", "pinned", "terminal", "done")
	assertOffers(t, h.complete(t, "pin", ""), "idle", "done")
	assertOffers(t, h.complete(t, "unpin", ""), "pinned")
	assertOffers(t, h.complete(t, "gc", ""), "done")
	assertOffers(t, h.complete(t, "where", ""), "idle", "pinned", "terminal", "done")
}

func TestCompletionSkipsNamesAlreadyGivenAndStopsAfterOneForSingleTargets(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.bg("bravo", "idle")}}

	assertOffers(t, h.complete(t, "stop", "alpha", ""), "bravo")
	assertOffers(t, h.complete(t, "attach", "alpha", ""))
}

func TestSkipCompletesSessionNames(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.bg("bravo", "idle")}}

	assertOffers(t, h.complete(t, "restart", "--skip", ""), "alpha", "bravo")
	assertOffers(t, h.complete(t, "pause", "--skip", "alpha", "--skip", ""), "bravo")
}

func TestResumeOffersParkedSessionsThenStoppedOnes(t *testing.T) {
	h := newHarness(t)
	a, gone := h.bg("alpha", "idle"), stopped(h.bg("gone", ""))
	h.agents.snapshots = [][]claude.Session{{a}}
	h.ok(t, "pause")
	h.world([]claude.Session{})
	h.agents.finished = []claude.Session{gone}

	assertOffers(t, h.complete(t, "resume", ""), "alpha", "gone")
	assertOffers(t, h.complete(t, "resume", "--forget", ""), "alpha")
	assertOffers(t, h.complete(t, "resume", "--skip", ""), "alpha")
}

func TestWhereAndLsCompleteMoreThanOneName(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.bg("bravo", "idle")}}

	assertOffers(t, h.complete(t, "where", "alpha", ""), "bravo")
	assertOffers(t, h.complete(t, "ls", "alpha", ""), "bravo")
	assertOffers(t, h.complete(t, "ls", "-o", ""), "text", "json", "yaml")
}
