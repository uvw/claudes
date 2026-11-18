package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func (h *harness) corruptPins(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(h.app.Jobs.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.app.Jobs.Dir, "pins.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBulkCommandsRefuseWhenPinsCannotBeRead(t *testing.T) {
	for _, args := range [][]string{{"restart"}, {"pause"}, {"gc"}} {
		t.Run(args[0], func(t *testing.T) {
			h := newHarness(t)
			old := stopped(h.bg("old", ""))
			old.StartedAt = h.now.Add(-30 * 24 * time.Hour).UnixMilli()
			h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
			h.agents.finished = []claude.Session{old}
			h.corruptPins(t)

			h.fails(t, args...)
			assertCalls(t, h)
			assertContains(t, h.err.String(), "cannot read pins, so cannot tell which sessions to protect")
		})
	}
}

func TestGCRefusesWhenTheParkFileCannotBeRead(t *testing.T) {
	h := newHarness(t)
	old := stopped(h.bg("old", ""))
	old.StartedAt = h.now.Add(-30 * 24 * time.Hour).UnixMilli()
	h.agents.finished = []claude.Session{old}
	os.MkdirAll(filepath.Dir(h.app.Park.Path), 0o755)
	os.WriteFile(h.app.Park.Path, []byte("{oops"), 0o644)

	h.fails(t, "gc")
	assertCalls(t, h)
	assertContains(t, h.err.String(), "cannot read the park file")
}

func TestGCReportsASessionItCouldNotRemove(t *testing.T) {
	h := newHarness(t)
	bad, good := stopped(h.bg("bad", "")), stopped(h.bg("good", ""))
	h.agents.finished = []claude.Session{bad, good}
	h.agents.failures = map[string]string{"rm " + bad.JobID: "worktree has changes"}

	h.fails(t, "gc", "bad", "good")
	assertCalls(t, h, "rm "+bad.JobID, "rm "+good.JobID)
	assertContains(t, h.out.String(), "removed    good")
	assertContains(t, h.err.String(), "bad: worktree has changes")
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

func TestPingRefusesAnInteractiveSession(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.tty("terminal", "busy")}}

	h.fails(t, "ping", "terminal")
	assertCalls(t, h)
	assertContains(t, h.err.String(), "only background sessions can be pinged")
}

func TestPingOfAStoppedSessionThatWillNotResumePrintsTheCommand(t *testing.T) {
	h := newHarness(t)
	old := stopped(h.bg("old", ""))
	h.agents.finished = []claude.Session{old}
	h.agents.failures = map[string]string{"resume " + old.SessionID + " in old prompt=Pick up where you left off.": "not logged in"}

	h.fails(t, "ping", "old")
	assertContains(t, h.err.String(), "not resumed (not logged in). Run: "+old.ManualCommand())
}

func TestHandOffRefusesAnInteractiveSession(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.tty("terminal", "idle")}}

	h.fails(t, "logs", "terminal")
	if len(h.execs) != 0 {
		t.Fatalf("execs %q", h.execs)
	}
	assertContains(t, h.err.String(), "claude logs works on background sessions")
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

func TestPassThroughThatCannotRunClaude(t *testing.T) {
	h := newHarness(t)
	h.app.Exec = func(string, ...string) error { return errors.New("executable file not found") }

	h.fails(t, "--some-agents-flag")
	assertContains(t, h.err.String(), "claudes: cannot run claude agents: executable file not found")
}

func TestSleepEndsEarlyWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if err := sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("err %v", err)
	}
}

func TestGCNeverFallsThroughToAnotherSession(t *testing.T) {
	h := newHarness(t)
	running := h.bg("api", "idle")
	other := stopped(h.bg("api-gw", ""))
	h.agents.snapshots = [][]claude.Session{{running}}
	h.agents.finished = []claude.Session{other}

	h.fails(t, "gc", "api")
	assertCalls(t, h)
	assertContains(t, h.err.String(), "api is still running: gc removes only finished background sessions. Stop it first")
}
