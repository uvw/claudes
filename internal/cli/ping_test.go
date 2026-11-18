package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

// A PID reused by another process after claude stop makes the wait fail,
// and the session stays stopped though it could have been resumed.
func TestAReusedPIDDoesNotBlockThePing(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "busy")
	h.agents.snapshots = [][]claude.Session{{s}}
	newer := start.Add(time.Minute)
	h.app.Alive = func(pid int, started time.Time) bool { return pid == s.PID && started.Equal(newer) }
	code := h.run("ping", "api", "carry on")
	if code != 0 {
		t.Fatalf("exit %d: the session was stopped and not resumed:\n%s\n%s", code, h.out, h.err)
	}
}

type slowStart struct{ *fakeAgents }

func (s slowStart) Resume(_ context.Context, sessionID, _, _ string, _ claude.NewJob) (string, error) {
	return "", s.do("resume", sessionID)
}

// claude --bg --resume returned, so the session is on its way. Telling the
// user it was not resumed, and to run claude --resume, starts a copy if it
// comes up late.
func TestASlowStartIsNotReportedAsNotResumed(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "busy")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.app.Agents = slowStart{h.agents}
	h.run("ping", "api", "carry on")
	msg := h.err.String()
	if strings.Contains(msg, "not resumed") || strings.Contains(msg, "claude --resume") {
		t.Fatalf("a resume that was issued is reported as not done:\n%s", msg)
	}
}

func TestPingStopsAndResumesWithTheText(t *testing.T) {
	h := newHarness(t)
	running, gone := h.bg("running", "idle"), stopped(h.bg("gone", ""))
	h.agents.snapshots = [][]claude.Session{{running}}
	h.agents.finished = []claude.Session{gone}

	h.ok(t, "ping", "running", "go on")
	h.ok(t, "ping", "gone", "go on")

	assertCalls(t, h,
		"stop "+running.JobID,
		"resume "+running.SessionID+" in running prompt=go on",
		"resume "+gone.SessionID+" in gone prompt=go on",
	)
}

func TestPingRefusesTheHost(t *testing.T) {
	h := newHarness(t)
	host := h.bg("host", "busy")
	h.app.HostID = host.SessionID
	h.agents.snapshots = [][]claude.Session{{host}}

	h.fails(t, "ping", "host")
	assertCalls(t, h)
}

func TestAPingWaitsForTheStoppedProcessToExitBeforeResuming(t *testing.T) {
	h := newHarness(t)
	s := h.bg("slow", "busy")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.lingers = map[int]int{s.PID: 2}

	h.ok(t, "ping", "slow", "go on")
	if h.lingers[s.PID] != 0 {
		t.Fatalf("resumed while the process was still alive: %d checks left", h.lingers[s.PID])
	}
	if len(h.sleeps) < 2 {
		t.Fatalf("waited %d times, want at least 2", len(h.sleeps))
	}
	assertCalls(t, h, "stop "+s.JobID, "resume "+s.SessionID+" in slow prompt=go on")
}

func TestPingTakesOneSessionAndAMessage(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}

	h.fails(t, "ping", "alpha", "beta", "sup")
	h.ok(t, "ping", "alpha", "sup dude")
	assertCalls(t, h, "stop "+a.JobID, "resume "+a.SessionID+" in alpha prompt=sup dude")
}

func TestPingReturnsOnlyOnceTheSessionShowsAsRunning(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	starting := s
	starting.PID, starting.State = 0, "working"
	// Stopped, then listed without a PID while it starts, then running.
	h.agents.snapshots = [][]claude.Session{{s}, {s}, {stopped(s)}, {starting}, {s}}

	h.ok(t, "ping", "api", "go on")
	if len(h.sleeps) == 0 {
		t.Fatal("ping returned before the session showed as running")
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

func TestPingWithoutAMessageSendsPingText(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "ping:\n  text: keep going\n")
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}

	h.ok(t, "ping", "api")
	assertCalls(t, h, "stop "+s.JobID, "resume "+s.SessionID+" in api prompt=keep going")
}
