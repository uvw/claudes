package cli

import (
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestSkipThatMatchesNothingStopsTheCommand(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("db-migration", "idle"), h.bg("other", "idle")}}

	h.fails(t, "restart", "--skip", "db-migrate")

	assertCalls(t, h)
	assertContains(t, h.err.String(), `--skip: no session matches "db-migrate"`)
}

func TestSkipMatchesLikeATargetAndIsListed(t *testing.T) {
	h := newHarness(t)
	db, other := h.bg("db-migration", "idle"), h.bg("other", "idle")
	h.agents.snapshots = [][]claude.Session{{db, other}}

	h.ok(t, "restart", "--skip", "DB")

	assertCalls(t, h, "respawn "+other.JobID)
	assertContains(t, h.out.String(), "Skipped, not restarted: db-migration")
}

func TestBulkCommandsLeaveABackgroundSessionWithoutAJobIDAlone(t *testing.T) {
	h := newHarness(t)
	a, odd := h.bg("alpha", "idle"), h.bg("odd", "idle")
	odd.JobID = ""
	h.agents.snapshots = [][]claude.Session{{a, odd}}

	h.ok(t, "restart")

	assertCalls(t, h, "respawn "+a.JobID)
	assertContains(t, h.out.String(), "gave no job id: odd")
}

func TestOutdatedWarnsWhenNoVersionCanBeRead(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}

	h.ok(t, "restart", "--outdated")

	assertCalls(t, h)
	assertContains(t, h.err.String(), "cannot read the Claude Code version of any session")
}

func TestOutdatedWithEverySessionPinnedDoesNotWarn(t *testing.T) {
	h := newHarness(t)
	p := h.bg("pinned", "idle")
	h.agents.snapshots = [][]claude.Session{{p}}
	h.pin(t, p)

	h.ok(t, "restart", "--outdated")

	assertCalls(t, h)
	if strings.Contains(h.err.String(), "cannot read the Claude Code version") {
		t.Fatalf("warned about versions with no session to check:\n%s", h.err)
	}
}

func TestRestartRespawnsRunningBackgroundSessionsByJobID(t *testing.T) {
	h := newHarness(t)
	a, gone, term := h.bg("alpha", "idle"), stopped(h.bg("gone", "")), h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{a, gone, term}}

	h.ok(t, "restart")

	assertCalls(t, h, "respawn "+a.JobID)
	if len(h.terminated) != 0 {
		t.Fatalf("terminated %v", h.terminated)
	}
	assertContains(t, h.out.String(), "Interactive sessions were left alone")
	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
}

func TestRestartLeavesPinnedSessionsAloneUnlessNamed(t *testing.T) {
	h := newHarness(t)
	a, p := h.bg("alpha", "idle"), h.bg("pinned", "idle")
	h.agents.snapshots = [][]claude.Session{{a, p}}
	h.pin(t, p)

	h.ok(t, "restart")
	assertCalls(t, h, "respawn "+a.JobID)
	assertContains(t, h.out.String(), "Pinned, not restarted: pinned")

	h.agents.calls = nil
	h.ok(t, "restart", "pinned")
	assertCalls(t, h, "respawn "+p.JobID)
}

func TestRestartTargetsByNamePartOrID(t *testing.T) {
	h := newHarness(t)
	db, reviewer, other := h.bg("db-upgrade-plan", "idle"), h.bg("code-reviewer", "idle"), h.bg("other", "idle")
	h.agents.snapshots = [][]claude.Session{{db, reviewer, other}}

	h.ok(t, "restart", "db", reviewer.JobID)

	assertCalls(t, h, "respawn "+db.JobID, "respawn "+reviewer.JobID)
}

func TestRestartTargetErrors(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("review-one", "idle"), h.bg("review-two", "idle"), h.tty("tty", "idle")}}

	for _, tc := range []struct{ arg, want string }{
		{"review", `"review" matches 2 sessions`},
		{"nope", `no session matches "nope"`},
		{"tty", "add --interactive"},
	} {
		h.err.Reset()
		h.fails(t, "restart", tc.arg)
		assertContains(t, h.err.String(), tc.want)
	}
	assertCalls(t, h)
}

func TestRestartPingStopsAndResumesBusySessions(t *testing.T) {
	h := newHarness(t)
	busy, idle := h.bg("busy", "busy"), h.bg("idle", "idle")
	h.agents.snapshots = [][]claude.Session{{busy, idle}}

	h.ok(t, "restart", "--ping")

	assertCalls(t, h,
		"stop "+busy.JobID,
		"resume "+busy.SessionID+" in busy prompt="+defaultPing,
		"respawn "+idle.JobID,
	)
	assertContains(t, h.out.String(), "restarted  busy and pinged")
}

func TestRestartPingText(t *testing.T) {
	h := newHarness(t)
	busy := h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{busy}}
	h.ok(t, "restart", "--ping-text", "carry on")
	assertCalls(t, h, "stop "+busy.JobID, "resume "+busy.SessionID+" in busy prompt=carry on")
}

func TestRestartHandlesTheHostLastAndNeverPingsIt(t *testing.T) {
	h := newHarness(t)
	host, other := h.bg("host", "busy"), h.bg("other", "busy")
	h.host(host)
	h.agents.snapshots = [][]claude.Session{{host, other}}

	h.ok(t, "restart", "--ping")

	assertCalls(t, h,
		"stop "+other.JobID,
		"resume "+other.SessionID+" in other prompt="+defaultPing,
		"respawn "+host.JobID,
	)
	assertContains(t, h.out.String(), "this session: restarted last, not pinged")
}

func TestRestartOutdatedOnlyTouchesOlderVersions(t *testing.T) {
	h := newHarness(t)
	old, current := h.bg("old", "idle"), h.bg("current", "idle")
	h.agents.snapshots = [][]claude.Session{{old, current}}
	writeState(t, h, old.JobID, "2.1.322")
	writeState(t, h, current.JobID, "2.1.325")

	h.ok(t, "restart", "--outdated")

	assertCalls(t, h, "respawn "+old.JobID)
}

// --interactive cannot bring an interactive session into --outdated, so the
// hint does not offer it.
func TestRestartOutdatedDoesNotOfferInteractive(t *testing.T) {
	h := newHarness(t)
	old, term := h.bg("old", "idle"), h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{old, term}}
	writeState(t, h, old.JobID, "2.1.322")

	h.ok(t, "restart", "--outdated", "--dry-run")
	out := h.out.String()
	assertContains(t, out, "Interactive sessions were left alone (--outdated never picks them).")
	if strings.Contains(out, "--interactive stops them") {
		t.Fatalf("--outdated offered --interactive:\n%s", out)
	}
}

func TestRestartInteractiveStopsAndPrintsTheCommand(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "busy")
	h.agents.snapshots = [][]claude.Session{{term}}

	h.ok(t, "restart", "--interactive", "--ping")

	if len(h.terminatedSince) != 1 || !h.terminatedSince[0].Equal(term.Started()) || term.Started().IsZero() {
		t.Fatalf("Terminate got the session start %v, want %v", h.terminatedSince, term.Started())
	}
	if !slices.Equal(h.terminated, []int{term.PID}) {
		t.Fatalf("terminated %v", h.terminated)
	}
	assertCalls(t, h)
	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
}

func TestRestartInteractiveBGResumesInTheBackground(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "busy")
	h.agents.snapshots = [][]claude.Session{{term}}

	h.ok(t, "restart", "--interactive-bg", "--ping")

	assertCalls(t, h, "resume "+term.SessionID+" in terminal prompt="+defaultPing)
}

func TestRestartDryRunAndConfirmChangeNothing(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}

	h.ok(t, "restart", "--dry-run")
	h.app.In = strings.NewReader("n\n")
	h.fails(t, "restart", "--confirm")
	assertCalls(t, h)
}

func TestRestartWaitPollsUntilIdle(t *testing.T) {
	h := newHarness(t)
	busy, idle := h.bg("alpha", "busy"), h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{busy}, {busy}, {idle}}

	h.ok(t, "restart", "--wait")

	if !slices.Equal(h.sleeps, []time.Duration{pollInterval, pollInterval}) {
		t.Fatalf("sleeps %v", h.sleeps)
	}
	assertCalls(t, h, "respawn "+idle.JobID)
}

func TestRestartWaitGivesUp(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "busy")}}

	h.fails(t, "restart", "--wait", "--timeout", "1m")

	assertCalls(t, h)
	assertContains(t, h.err.String(), "still busy after 1m0s")
}

func TestRestartFailureIsReportedAndTheRestContinue(t *testing.T) {
	h := newHarness(t)
	bad, good := h.bg("bad", "idle"), h.bg("good", "idle")
	h.agents.snapshots = [][]claude.Session{{bad, good}}
	h.agents.failures = map[string]string{"respawn " + bad.JobID: "boom"}

	h.fails(t, "restart")

	assertCalls(t, h, "respawn "+bad.JobID, "respawn "+good.JobID)
	assertContains(t, h.err.String(), "bad: boom")
}

// restart finishes the session in hand, names the rest, and exits 130 on a
// SIGTERM, as it does on Ctrl-C.
func TestASIGTERMDuringRestartReportsTheRest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM to send")
	}
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}
	h.app.Interrupt = signalContext
	var once sync.Once
	h.agents.beforeRespawn = func(string) {
		once.Do(func() { _ = self.Signal(syscall.SIGTERM) })
		// The signal reaches the run's context on another goroutine.
		time.Sleep(100 * time.Millisecond)
	}

	if code := h.run("restart"); code != exitInterrupted {
		t.Fatalf("exit %d, want %d:\n%s\n%s", code, exitInterrupted, h.out, h.err)
	}
	assertCalls(t, h, "respawn "+a.JobID)
	assertContains(t, h.out.String(), "restarted  alpha")
	assertContains(t, h.err.String(), "Not reached after Ctrl-C, left as they were: beta")
}
