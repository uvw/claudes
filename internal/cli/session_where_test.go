package cli

import (
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestSessionWhereOneRefusesSeveralSessions(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "alpha-one", h.folder("a"), time.Hour)
	h.transcript(t, "alpha-two", h.folder("b"), time.Hour)

	h.exits(t, 2, "session", "where", "--one", "alpha")
	assertContains(t, h.err.String(), "matches 2 sessions")
	h.err.Reset()
	h.exits(t, 2, "session", "where", "--one", "alpha-one", "alpha-two")
	assertContains(t, h.err.String(), "2 sessions match")
	h.err.Reset()
	h.ok(t, "session", "where", "alpha")
	assertContains(t, h.out.String(), "alpha-one")
	assertContains(t, h.out.String(), "alpha-two")
}

func TestSessionWhereTellsSessionsWithOneNameApart(t *testing.T) {
	h := newHarness(t)
	a := h.transcript(t, "plan", h.folder("one"), time.Hour)
	b := h.transcript(t, "plan", h.folder("two"), 2*time.Hour)

	h.ok(t, "session", "where", "plan")
	for _, id := range []string{a, b} {
		assertContains(t, h.out.String(), claude.ShortID(id))
	}
}
