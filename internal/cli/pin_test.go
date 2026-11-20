package cli

import (
	"slices"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestPinAndUnpinWriteAgentViewPins(t *testing.T) {
	h := newHarness(t)
	a, term := h.bg("alpha", "idle"), h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{a, term}}

	h.ok(t, "pin", "alpha")
	pins, _ := h.app.Jobs.Pins()
	if !slices.Equal(pins, []string{a.JobID}) {
		t.Fatalf("pins %v", pins)
	}
	h.fails(t, "pin", "terminal")
	h.ok(t, "unpin", "alpha")
	if pins, _ = h.app.Jobs.Pins(); len(pins) != 0 {
		t.Fatalf("pins after unpin %v", pins)
	}
}
