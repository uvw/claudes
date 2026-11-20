package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
)

func parked(t *testing.T, h *harness) []park.Entry {
	t.Helper()
	entries, err := h.app.Park.Load()
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func isParked(t *testing.T, h *harness, s claude.Session) bool {
	t.Helper()
	return slices.ContainsFunc(parked(t, h), func(e park.Entry) bool { return e.SessionID == s.SessionID })
}

func TestRestartPrintsEverythingBeforeTheHostStops(t *testing.T) {
	h := newHarness(t)
	host, term, bad := h.bg("host", "idle"), h.tty("terminal", "idle"), h.bg("bad", "idle")
	h.host(host)
	h.agents.snapshots = [][]claude.Session{{host, term, bad}}
	h.agents.failures = map[string]string{"respawn " + bad.JobID: "boom"}

	h.fails(t, "restart", "--interactive")

	assertCalls(t, h, "respawn "+bad.JobID, "respawn "+host.JobID)
	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
	assertContains(t, h.err.String(), "bad: boom")
	assertContains(t, h.out.String(), "Restarting this session (host) last. Output ends here.")
}

func TestRestartParksAnInteractiveHostAndNeverResumesIt(t *testing.T) {
	h := newHarness(t)
	host := h.tty("host", "busy")
	h.host(host)
	h.agents.snapshots = [][]claude.Session{{host}}

	h.ok(t, "restart", "--interactive-bg", "--ping")

	assertCalls(t, h)
	if !slices.Equal(h.terminated, []int{host.PID}) {
		t.Fatalf("terminated %v", h.terminated)
	}
	assertContains(t, h.out.String(), "claude --resume "+host.SessionID)
	if !isParked(t, h, host) {
		t.Fatal("the host is not parked")
	}
}

func TestPauseReportsEverythingBeforeTheHostStops(t *testing.T) {
	h := newHarness(t)
	host, a := h.bg("host", "idle"), h.bg("alpha", "idle")
	h.host(host)
	h.agents.snapshots = [][]claude.Session{{host, a}}

	h.ok(t, "pause")

	assertCalls(t, h, "stop "+a.JobID, "stop "+host.JobID)
	assertContains(t, h.out.String(), "Parked 2 session(s)")
	assertContains(t, h.out.String(), "Pausing this session (host) last. Output ends here.")
	if len(parked(t, h)) != 2 {
		t.Fatalf("parked %+v", parked(t, h))
	}
}

func TestStopReportsFailuresBeforeTheHost(t *testing.T) {
	h := newHarness(t)
	host, bad := h.bg("host", "idle"), h.bg("bad", "idle")
	h.host(host)
	h.agents.snapshots = [][]claude.Session{{host, bad}}
	h.agents.failures = map[string]string{"stop " + bad.JobID: "boom"}

	h.fails(t, "stop", "host", "bad")

	assertCalls(t, h, "stop "+bad.JobID, "stop "+host.JobID)
	assertContains(t, h.err.String(), "bad: boom")
}

func TestRestartPingsSessionsThatAreWaitingOrRunningAShell(t *testing.T) {
	h := newHarness(t)
	waiting, shell := h.bg("waiting", "waiting"), h.bg("shell", "shell")
	h.agents.snapshots = [][]claude.Session{{waiting, shell}}

	h.ok(t, "restart", "--ping", "--wait")

	if len(h.sleeps) != 0 {
		t.Fatalf("waited for sessions that are not busy: %v", h.sleeps)
	}
	assertCalls(t, h,
		"stop "+waiting.JobID, "resume "+waiting.SessionID+" in waiting prompt="+defaultPing,
		"stop "+shell.JobID, "resume "+shell.SessionID+" in shell prompt="+defaultPing,
	)
}

func TestRestartPingWaitsForTheStopToLand(t *testing.T) {
	h := newHarness(t)
	busy := h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{busy}, {busy}, {stopped(busy)}}
	h.agents.lagStop = true

	h.ok(t, "restart", "--ping")

	if !slices.Equal(h.sleeps, []time.Duration{checkEvery}) {
		t.Fatalf("sleeps %v", h.sleeps)
	}
	assertCalls(t, h, "stop "+busy.JobID, "resume "+busy.SessionID+" in busy prompt="+defaultPing)
}

func TestRestartPingDoesNotResumeASessionThatWillNotStop(t *testing.T) {
	h := newHarness(t)
	busy := h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{busy}}
	h.agents.lagStop = true

	h.fails(t, "restart", "--ping")

	assertCalls(t, h, "stop "+busy.JobID)
	assertContains(t, h.err.String(), "still running")
}

func TestRestartPingParksTheSessionUntilItIsBack(t *testing.T) {
	h := newHarness(t)
	busy := h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{busy}}
	h.agents.beforeStop = func(string) {
		if !isParked(t, h, busy) {
			t.Error("stopped before the session was parked")
		}
	}

	h.ok(t, "restart", "--ping")
	if len(parked(t, h)) != 0 {
		t.Fatalf("still parked: %+v", parked(t, h))
	}
}

func TestRestartPingThatFailsToResumeLeavesTheSessionParked(t *testing.T) {
	h := newHarness(t)
	busy := h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{busy}}
	h.agents.failures = map[string]string{"resume " + busy.SessionID + " in busy prompt=" + defaultPing: "boom"}

	h.fails(t, "restart", "--ping")

	if !isParked(t, h, busy) {
		t.Fatal("the session is not parked")
	}
	entry := parked(t, h)[0]
	if !entry.WasBusy {
		t.Fatal("the parked entry will not be pinged on resume")
	}
	assertContains(t, h.err.String(), "claudes resume "+busy.JobID)
}

func TestRestartInteractiveBGFailureKeepsTheResumeCommand(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{term}}
	h.agents.failures = map[string]string{"resume " + term.SessionID + " in terminal prompt=": "boom"}

	h.fails(t, "restart", "--interactive-bg")

	assertContains(t, h.err.String(), "claude --resume "+term.SessionID)
	if !isParked(t, h, term) {
		t.Fatal("the session is not parked")
	}
}

func TestRestartDoesNotStopASessionWhoseFolderIsGone(t *testing.T) {
	h := newHarness(t)
	busy, term := h.bg("busy", "busy"), h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{busy, term}}
	for _, s := range []claude.Session{busy, term} {
		if err := os.RemoveAll(s.Cwd); err != nil {
			t.Fatal(err)
		}
	}

	h.fails(t, "restart", "--ping", "--interactive-bg")

	assertCalls(t, h)
	if len(h.terminated) != 0 {
		t.Fatalf("terminated %v", h.terminated)
	}
	assertContains(t, h.err.String(), "so it was not stopped")
}

func TestRestartReportsASessionThatDidNotComeBack(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}, {stopped(a)}}

	h.fails(t, "restart")

	assertContains(t, h.err.String(), "alpha: respawned, but not running")
}

func TestPauseKeepsASessionWhoseStopFailedButTookEffect(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}
	h.agents.failures = map[string]string{"stop " + a.JobID: "context canceled"}
	h.agents.beforeStop = func(id string) { h.agents.markDown(id, true) }

	h.fails(t, "pause")

	if !isParked(t, h, a) {
		t.Fatal("dropped a session that may be stopped")
	}
}

func TestPauseListsInteractiveSessionsItLeftAlone(t *testing.T) {
	h := newHarness(t)
	a, term := h.bg("alpha", "idle"), h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{a, term}}

	h.ok(t, "pause")

	assertContains(t, h.out.String(), "Interactive sessions were left alone")
	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
}

func TestPauseAndResumeDryRunsChangeNothing(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}

	h.ok(t, "pause", "--dry-run")
	assertCalls(t, h)
	if len(parked(t, h)) != 0 {
		t.Fatal("dry run parked a session")
	}

	h.ok(t, "pause")
	h.world([]claude.Session{})
	h.ok(t, "resume", "--dry-run")
	assertCalls(t, h)
	if len(parked(t, h)) != 1 {
		t.Fatal("dry run changed the park file")
	}
}

func TestResumeKeepsAnInteractiveSessionUntilItRuns(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{term}}
	h.ok(t, "pause", "--interactive")
	h.world([]claude.Session{})

	h.ok(t, "resume")
	h.ok(t, "resume")
	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
	if !isParked(t, h, term) {
		t.Fatal("dropped before it was seen running")
	}

	h.world([]claude.Session{term})
	h.ok(t, "resume")
	if isParked(t, h, term) {
		t.Fatal("still parked after it was seen running")
	}
}

func TestResumeForgetDropsAnEntry(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("bravo", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}
	h.ok(t, "pause")
	h.world([]claude.Session{})

	h.ok(t, "resume", "--forget", a.JobID)

	assertCalls(t, h)
	if isParked(t, h, a) || !isParked(t, h, b) {
		t.Fatalf("parked %+v", parked(t, h))
	}
}

func TestAPromptThatLooksLikeAnOptionStopsNothing(t *testing.T) {
	h := newHarness(t)
	busy := h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{busy}}

	// pflag gives -m and --message the next argument even when it starts with
	// a dash, so each form reaches the prompt check as the message.
	for _, args := range [][]string{
		{"restart", "--ping-text", "--dangerously-skip-permissions"},
		{"ping", "busy", "-m", "--dangerously-skip-permissions"},
		{"ping", "busy", "-m=--dangerously-skip-permissions"},
		{"ping", "busy", "--message", "--dangerously-skip-permissions"},
		{"ping", "busy", "--message=--dangerously-skip-permissions"},
	} {
		h.err.Reset()
		h.fails(t, args...)
		assertContains(t, h.err.String(), `"--dangerously-skip-permissions" starts with -`)
	}
	// After --, it is a target, and it names no session.
	h.exits(t, exitNoMatch, "ping", "busy", "--", "--dangerously-skip-permissions")
	assertCalls(t, h)
}

func TestPauseCountsAFailedStopThatStillTookEffect(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}
	h.agents.failures = map[string]string{"stop " + b.JobID: "timed out"}
	// The stop lands, then claude reports a failure anyway.
	h.agents.beforeStop = func(id string) { h.agents.markDown(id, true) }

	h.fails(t, "pause")
	assertContains(t, h.out.String(), "Parked 2 session(s)")
	assertContains(t, h.err.String(), "it stays parked")
}

func TestCtrlCFinishesTheSessionInHandAndNamesTheRest(t *testing.T) {
	h := newHarness(t)
	a, b, c := h.bg("alpha", "idle"), h.bg("beta", "idle"), h.bg("gamma", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b, c}}
	// The Ctrl-C arrives while alpha is being respawned.
	h.agents.beforeRespawn = func(string) { h.interrupt() }

	if code := h.run("restart"); code != 130 {
		t.Fatalf("exit %d", code)
	}
	assertCalls(t, h, "respawn "+a.JobID)
	assertContains(t, h.out.String(), "restarted  alpha")
	assertContains(t, h.err.String(), "Not reached after Ctrl-C, left as they were: beta, gamma")
	assertContains(t, h.err.String(), "interrupted")
}

func TestRestartLooksAgainBeforeActingOnAStaleListing(t *testing.T) {
	h := newHarness(t)
	busy, later := h.bg("busy", "busy"), h.bg("later", "idle")
	laterBusy := later
	laterBusy.Status = "busy"
	// The second listing, after busy's ping, shows later mid-turn.
	h.agents.snapshots = [][]claude.Session{{busy, later}, {stopped(busy), laterBusy}, {busy, laterBusy}}
	// A stop takes real time, so the first listing is stale by later's turn.
	h.agents.beforeStop = func(string) { h.now = h.now.Add(10 * time.Second) }

	h.ok(t, "restart", "--ping")
	assertCalls(t, h, "stop "+busy.JobID, "resume "+busy.SessionID+" in busy prompt="+defaultPing,
		"stop "+later.JobID, "resume "+later.SessionID+" in later prompt="+defaultPing)
	if slices.Contains(h.agents.calls, "respawn "+later.JobID) {
		t.Fatalf("later was respawned while busy, without a ping: %q", h.agents.calls)
	}
}

func TestPlainRestartWarnsAboutMidTurnSessions(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "waiting")}}
	h.ok(t, "restart", "--dry-run")
	assertContains(t, h.out.String(), "waiting: its turn ends, and --ping would carry it on")
}

func TestPauseLeavesASessionWhoseFolderIsGone(t *testing.T) {
	h := newHarness(t)
	gone := h.bg("gone", "idle")
	gone.Cwd = filepath.Join(h.root, "deleted")
	h.agents.snapshots = [][]claude.Session{{gone}}

	h.fails(t, "pause")
	assertCalls(t, h)
	assertContains(t, h.err.String(), "is gone, so it was not stopped")
	if len(parked(t, h)) != 0 {
		t.Fatalf("parked %+v", parked(t, h))
	}
}
