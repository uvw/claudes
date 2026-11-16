package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/teams"
)

// A record that yields two events is one transcript record; -o json promises
// "the transcript records themselves, one per line".
func TestLogsJSONPrintsEachRecordOnce(t *testing.T) {
	h := newHarness(t)
	s := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s,
		`{"type":"user","message":{"content":"<teammate-message teammate_id=\"a\">hi</teammate-message><teammate-message teammate_id=\"b\">yo</teammate-message>"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
	)
	h.ok(t, "logs", "lead", "--tail", "10", "-o", "json")
	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("2 records printed as %d lines:\n%s", len(lines), h.out)
	}
}

// --tail N counts events, so a record that yields several events can push
// --tail 1 -o json to print the same record, and never the one before it.
func TestLogsTailCountsRecordsInJSON(t *testing.T) {
	h := newHarness(t)
	s := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s,
		`{"type":"user","message":{"content":"first"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
	)
	h.ok(t, "logs", "lead", "--tail", "2", "-o", "json")
	if !strings.Contains(h.out.String(), "first") {
		t.Fatalf("--tail 2 -o json left out the second-to-last record:\n%s", h.out)
	}
}

// Teammate names, task owners, and member folders come from config.json and
// task files, and reach the terminal without shell.Printable.
func TestTeamLsStripsControlCharacters(t *testing.T) {
	h := newHarness(t)
	h.app.colors = style.Palette{}
	lead := h.bg("lead", "busy")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead, "evil\x1b]0;pwned\x07\x1b[2J")
	tasks := filepath.Join(h.app.Teams.Tasks, teams.Name(lead.SessionID))
	writeFile(t, filepath.Join(tasks, "1.json"), `{"id":"1","subject":"x","status":"in_progress","owner":"own\u001b[31mer"}`)

	h.ok(t, "team", "ls")
	if strings.ContainsRune(h.out.String(), 0x1b) {
		t.Errorf("team ls printed an escape sequence: %q", h.out.String())
	}
	h.out.Reset()
	h.ok(t, "restart", "--ping", "lead", "--dry-run")
	if strings.ContainsRune(h.out.String(), 0x1b) {
		t.Errorf("restart --dry-run printed an escape sequence: %q", h.out.String())
	}
	h.out.Reset()
	h.err.Reset()
	h.ok(t, "stop", "lead")
	if strings.ContainsRune(h.err.String()+h.out.String(), 0x1b) {
		t.Fatalf("stop printed an escape sequence: %q", h.err.String())
	}
}

// gc protects parked sessions, so claudes resume finds them as they were.
// gc --teams does not: a paused lead's task list is removed after a week.
func TestGCTeamsKeepsAParkedSessionsTaskList(t *testing.T) {
	h := newHarness(t)
	paused := stopped(h.bg("paused", ""))
	h.agents.snapshots = [][]claude.Session{{}}
	h.agents.finished = []claude.Session{paused}
	if err := h.app.Park.Update(func(e []park.Entry) []park.Entry {
		return park.Merge(e, []park.Entry{park.FromSession(paused, h.now.Format(time.RFC3339))})
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

// Named subagents of a session that leads no team are taken for teammates.
func TestLogsLeadSlashNameNeedsATeam(t *testing.T) {
	h := newHarness(t)
	solo := h.bg("solo", "busy")
	h.agents.snapshots = [][]claude.Session{{solo}}
	path := h.logLines(t, solo, `{"type":"user","message":{"content":"go"}}`)
	writeFile(t, filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-adeps-d27ce76e9061ede7.jsonl"),
		`{"type":"assistant","message":{"content":[{"type":"text","text":"a subagent"}]}}`+"\n")
	code := h.run("logs", "solo/deps")
	if code == 0 {
		t.Fatalf("logs solo/deps showed a subagent as a teammate of a session with no team:\n%s", h.out)
	}
}

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

func followWith(t *testing.T, h *harness, steps ...func(path string)) string {
	t.Helper()
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	path := h.logLines(t, s, `{"type":"user","message":{"content":"old"}}`)
	n := 0
	h.app.Sleep = func(ctx context.Context, _ time.Duration) error {
		if n == len(steps) {
			h.interrupt()
			return ctx.Err()
		}
		steps[n](path)
		n++
		return nil
	}
	h.ok(t, "logs", "-f", "api", "--tail", "0")
	return h.out.String()
}

func appendTo(t *testing.T, path, text string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(f, text)
	f.Close()
}

func TestFollowPrintsALineSplitAcrossPollsOnce(t *testing.T) {
	h := newHarness(t)
	out := followWith(t, h,
		func(p string) { appendTo(t, p, `{"type":"user","message":{"cont`) },
		func(p string) { appendTo(t, p, `ent":"split"}}`+"\n") },
		func(p string) {},
	)
	if strings.Count(out, "split") != 1 {
		t.Fatalf("output:\n%s", out)
	}
}

// A transcript replaced by a longer one is read from the old offset, mid-line.
func TestFollowNoticesAReplacedTranscript(t *testing.T) {
	h := newHarness(t)
	out := followWith(t, h,
		func(p string) {
			os.WriteFile(p, []byte(`{"type":"user","message":{"content":"replaced one"}}`+"\n"+`{"type":"user","message":{"content":"replaced two"}}`+"\n"), 0o644)
		},
		func(p string) {},
	)
	if !strings.Contains(out, "replaced one") {
		t.Fatalf("skipped the first line of the new transcript:\n%s\n%s", out, h.err)
	}
}

type panicky struct{ *fakeAgents }

func (p panicky) Respawn(context.Context, string) error { panic("boom") }

func TestAPanicFreesTheRunLock(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}
	h.app.Agents = panicky{h.agents}
	func() {
		defer func() { _ = recover() }()
		h.run("restart", "api")
	}()
	if _, err := os.Stat(h.app.LockPath); err == nil {
		t.Fatal("the run lock outlived a panic")
	}
}

// docs/reference.md: exit 4 is "Refused before any change: the wrong kind of
// session, or a failed safety check".
func TestSafetyRefusalsExitFour(t *testing.T) {
	h := newHarness(t)
	host := h.bg("host", "busy")
	live := h.bg("live", "busy")
	h.app.HostID = host.SessionID
	h.agents.snapshots = [][]claude.Session{{host, live}}
	h.transcriptAs(t, live.SessionID, "live", live.Cwd, time.Minute)
	for _, args := range [][]string{
		{"ping", "host", "hi"},
		{"session", "resume", "live"},
		{"session", "mv", "live", h.folder("other")},
		{"gc", "live"},
	} {
		os.MkdirAll(h.folder("other"), 0o755)
		h.err.Reset()
		if code := h.run(args...); code != exitRefused {
			t.Errorf("%v exits %d: %s", args, code, strings.TrimSpace(h.err.String()))
		}
	}
}

// Session names often hold a slash, like a branch name.
func TestLogsOfASessionNamedWithASlash(t *testing.T) {
	h := newHarness(t)
	s := h.bg("fix/auth", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s, `{"type":"user","message":{"content":"go"}}`)
	h.ok(t, "where", "fix/auth")
	if code := h.run("logs", "fix/auth"); code != 0 {
		t.Errorf("logs fix/auth exits %d: %s", code, h.err)
	}
	h.err.Reset()
	if code := h.run("logs", "fix/auth", "--tail", "5"); code != 0 {
		t.Errorf("logs fix/auth --tail 5 exits %d: %s", code, h.err)
	}
}

type slowStart struct{ *fakeAgents }

func (s slowStart) Resume(_ context.Context, sessionID, cwd, prompt, name string) (string, error) {
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
