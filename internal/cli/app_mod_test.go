package cli

import (
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestModStatusIgnoresAPresenceFromBeforeTheSessionStarted(t *testing.T) {
	h := newHarness(t)
	s := h.bg("restarted", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.now = h.now.Add(-2 * time.Hour)
	h.loaded(t, s)
	h.now = h.now.Add(2 * time.Hour)
	if got := h.app.modLoaded(s); got != "" {
		t.Fatalf("took an old presence as loaded: %q", got)
	}
}
