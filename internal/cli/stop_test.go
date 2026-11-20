package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestStopParksAnInteractiveSessionAndPrintsHowToResumeIt(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{term}}

	h.ok(t, "stop", "terminal")

	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
	if entries, _ := h.app.Park.Load(); len(entries) != 1 || entries[0].SessionID != term.SessionID {
		t.Fatalf("parked %+v", entries)
	}
}

func TestStopUsesTheJobIDOrASignal(t *testing.T) {
	h := newHarness(t)
	a, term := h.bg("alpha", "idle"), h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{a, term}}

	h.ok(t, "stop", "alpha", "terminal")

	assertCalls(t, h, "stop "+a.JobID)
	if !slices.Equal(h.terminated, []int{term.PID}) {
		t.Fatalf("terminated %v", h.terminated)
	}
}

func TestStopReturnsOnlyOnceTheProcessHasExited(t *testing.T) {
	h := newHarness(t)
	s := h.bg("slow", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.lingers = map[int]int{s.PID: 3}

	h.ok(t, "stop", "slow")
	if h.lingers[s.PID] != 0 {
		t.Fatalf("stop returned while the process still ran: %d checks left", h.lingers[s.PID])
	}
}

func TestStopDoesTheHostLast(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		t.Run(map[bool]string{false: "background", true: "interactive"}[interactive], func(t *testing.T) {
			h := newHarness(t)
			other := h.bg("other", "idle")
			host := h.bg("host", "idle")
			if interactive {
				host = h.tty("host", "idle")
			}
			h.agents.snapshots = [][]claude.Session{{host, other}}
			h.host(host)

			h.ok(t, "stop", "host", "other")

			out := h.out.String()
			otherAt, hostAt := strings.Index(out, "stopped    other"), strings.Index(out, "Stopping this session (host) last.")
			if otherAt < 0 || hostAt < 0 || otherAt > hostAt {
				t.Fatalf("the host was not last:\n%s", out)
			}
			entries, _ := h.app.Park.Load()
			if interactive {
				if !slices.Equal(h.terminated, []int{host.PID}) || len(entries) != 1 || entries[0].SessionID != host.SessionID {
					t.Fatalf("terminated %v, parked %+v", h.terminated, entries)
				}
				assertContains(t, out, host.ManualCommand())
				assertCalls(t, h, "stop "+other.JobID)
			} else {
				if len(entries) != 0 {
					t.Fatalf("parked %+v", entries)
				}
				assertCalls(t, h, "stop "+other.JobID, "stop "+host.JobID)
			}
		})
	}
}
