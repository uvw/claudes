package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/jobs"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/safefile"
	"github.com/uvw/claudes/internal/teams"
	"github.com/uvw/claudes/internal/transcripts"
)

var jobIDPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

// fakeAgents stands in for the claude command line. Like the real one, stop,
// respawn, and rm accept only a known 8-character job id. Each call to
// Sessions moves to the next snapshot, and the last one repeats. A session
// stopped through the fake shows as stopped until it is resumed, respawned,
// or the fake moves to a new snapshot.
type fakeAgents struct {
	snapshots [][]claude.Session
	finished  []claude.Session
	calls     []string
	failures  map[string]string
	version   string
	// help replaces the output of `claude [command] --help`, by command.
	help      map[string]string
	resumeOut string
	seen      map[string]bool
	down      map[string]bool
	// hostJob is the host session's job id; touching it calls onHost.
	hostJob string
	onHost  func()
	// beforeStop runs before every stop, to check what is on disk by then.
	beforeStop func(sessionID string)
	// beforeRespawn runs before every respawn.
	beforeRespawn func(sessionID string)
	// lagStop leaves a stopped session running until the next snapshot, as
	// when `claude stop` returns before the job is down.
	lagStop bool
	// respawns counts each session's respawns: a respawned session runs as a
	// new process, so it shows a new PID.
	respawns map[string]int
	// resumed are sessions Resume brought back that no snapshot lists.
	resumed map[string]claude.Session
}

func (f *fakeAgents) current() []claude.Session {
	if len(f.snapshots) == 0 {
		return nil
	}
	s := f.snapshots[0]
	for _, x := range s {
		if f.seen == nil {
			f.seen = map[string]bool{}
		}
		f.seen[x.JobID] = true
	}
	if len(f.snapshots) > 1 {
		f.snapshots = f.snapshots[1:]
		f.down = nil
	}
	out := slices.Clone(s)
	for i := range out {
		if n := f.respawns[out[i].SessionID]; n > 0 && out[i].PID > 0 {
			out[i].PID += 100000 * n
		}
		if f.down[out[i].SessionID] {
			out[i].PID, out[i].Status, out[i].State = 0, "", "stopped"
		}
	}
	return out
}

func (f *fakeAgents) Sessions(_ context.Context, all bool) ([]claude.Session, error) {
	if msg, ok := f.failures["list"]; ok {
		return nil, errors.New(msg)
	}
	s := f.current()
	if all {
		s = append(s, f.finished...)
	}
	// A session resumed from outside the scripted snapshots runs from then on,
	// as it does in Claude Code.
	for id, r := range f.resumed {
		i := slices.IndexFunc(s, func(x claude.Session) bool { return x.SessionID == id })
		switch {
		case i < 0:
			s = append(s, r)
		case s[i].PID == 0:
			s[i] = r
		}
	}
	return s, nil
}

// sessionID finds the session id for a job id in any snapshot.
func (f *fakeAgents) sessionID(jobID string) string {
	for _, snap := range f.snapshots {
		for _, s := range snap {
			if s.JobID == jobID {
				return s.SessionID
			}
		}
	}
	return ""
}

// listed reports whether the current snapshot lists the session as running,
// without moving on to the next snapshot.
func (f *fakeAgents) listed(sessionID string) bool {
	return len(f.snapshots) > 0 && slices.ContainsFunc(f.snapshots[0], func(x claude.Session) bool {
		return x.SessionID == sessionID && x.PID > 0
	})
}

func (f *fakeAgents) markDown(sessionID string, down bool) {
	if f.down == nil {
		f.down = map[string]bool{}
	}
	if down {
		f.down[sessionID] = true
	} else {
		delete(f.down, sessionID)
	}
}

func (f *fakeAgents) known(jobID string) bool {
	if f.seen[jobID] {
		return true
	}
	for _, snap := range f.snapshots {
		for _, s := range snap {
			if s.JobID == jobID {
				return true
			}
		}
	}
	for _, s := range f.finished {
		if s.JobID == jobID {
			return true
		}
	}
	return false
}

func (f *fakeAgents) do(op, id string) error {
	f.calls = append(f.calls, op+" "+id)
	if msg, ok := f.failures[op+" "+id]; ok {
		return errors.New(msg)
	}
	return nil
}

func (f *fakeAgents) job(op, jobID string) error {
	if !jobIDPattern.MatchString(jobID) || !f.known(jobID) {
		f.calls = append(f.calls, op+" "+jobID)
		return fmt.Errorf("No job matching '%s'", jobID)
	}
	return f.do(op, jobID)
}

func (f *fakeAgents) touch(jobID string) {
	if jobID != "" && jobID == f.hostJob && f.onHost != nil {
		f.onHost()
	}
}

func (f *fakeAgents) Respawn(_ context.Context, jobID string) error {
	if f.beforeRespawn != nil {
		f.beforeRespawn(f.sessionID(jobID))
	}
	err := f.job("respawn", jobID)
	if err == nil {
		f.markDown(f.sessionID(jobID), false)
		if f.respawns == nil {
			f.respawns = map[string]int{}
		}
		f.respawns[f.sessionID(jobID)]++
	}
	f.touch(jobID)
	return err
}

func (f *fakeAgents) Stop(_ context.Context, jobID string) error {
	if f.beforeStop != nil {
		f.beforeStop(f.sessionID(jobID))
	}
	err := f.job("stop", jobID)
	if err == nil && !f.lagStop {
		f.markDown(f.sessionID(jobID), true)
	}
	f.touch(jobID)
	return err
}

func (f *fakeAgents) Remove(_ context.Context, jobID string) error { return f.job("rm", jobID) }

func (f *fakeAgents) Resume(_ context.Context, sessionID, cwd, prompt, name string) (string, error) {
	call := sessionID + " in " + filepath.Base(cwd) + " prompt=" + prompt
	if name != "" {
		call += " name=" + name
	}
	err := f.do("resume", call)
	if err == nil {
		f.markDown(sessionID, false)
		if !f.listed(sessionID) {
			if f.resumed == nil {
				f.resumed = map[string]claude.Session{}
			}
			f.resumed[sessionID] = claude.Session{
				JobID: claude.ShortID(sessionID), SessionID: sessionID, Cwd: cwd,
				Kind: "background", State: "done", Status: "idle", PID: 7000 + len(f.resumed),
			}
		}
	}
	return f.resumeOut, err
}

func (f *fakeAgents) CheckPrompt(prompt string) error { return claude.CheckOption(prompt) }

func (f *fakeAgents) Help(_ context.Context, command string) (string, error) {
	if text, ok := f.help[command]; ok {
		return text, nil
	}
	name := "help-2.1.325.txt"
	if command != "" {
		name = command + "-help-2.1.325.txt"
	}
	data, err := os.ReadFile(filepath.Join("..", "compat", "testdata", name))
	return string(data), err
}

func (f *fakeAgents) Version(context.Context) (string, error) {
	if f.version == "" {
		return "2.1.325", nil
	}
	return f.version, nil
}

// cutWriter drops everything written after cut is set, the way output ends
// when the session running claudes stops.
type cutWriter struct {
	bytes.Buffer
	cut bool
}

func (w *cutWriter) Write(p []byte) (int, error) {
	if w.cut {
		return len(p), nil
	}
	return w.Buffer.Write(p)
}

type harness struct {
	app        *App
	agents     *fakeAgents
	out, err   *cutWriter
	root       string
	terminated []int
	execs      [][]string
	chdirs     []string
	sleeps     []time.Duration
	hostPID    int
	now        time.Time
	// interrupt cancels the running command's context, as a first Ctrl-C does.
	interrupt context.CancelFunc
	// lingers is how many more checks each PID stays alive after its stop.
	lingers map[int]int
}

// start is when every test's clock starts. Each harness keeps its own clock,
// so a test that sleeps moves no other test's time.
var start = time.Date(2026, 11, 13, 17, 0, 0, 0, time.UTC)

func newHarness(t *testing.T, snapshots ...[]claude.Session) *harness {
	t.Helper()
	root := t.TempDir()
	// Claude Code creates the jobs folder; a missing one means the wrong folder.
	if err := os.MkdirAll(filepath.Join(root, "jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &harness{agents: &fakeAgents{snapshots: snapshots}, out: &cutWriter{}, err: &cutWriter{}, root: root, now: start}
	h.app = &App{
		Agents:   h.agents,
		Jobs:     jobs.Store{Dir: filepath.Join(h.root, "jobs"), LockAttempts: 3, LockDelay: time.Millisecond},
		Park:     park.File{Path: filepath.Join(h.root, "state", "parked.json")},
		LockPath: filepath.Join(h.root, "state", "run.lock"),
		Out:      h.out,
		Err:      h.err,
		In:       strings.NewReader(""),
		Now:      func() time.Time { return h.now },
		Sleep: func(_ context.Context, d time.Duration) error {
			h.sleeps = append(h.sleeps, d)
			h.now = h.now.Add(d)
			return nil
		},
		Started: func(pid int) (time.Time, bool) { return start, true },
		Alive: func(pid int, _ time.Time) bool {
			if h.lingers[pid] > 0 {
				h.lingers[pid]--
				return true
			}
			return false
		},
		Terminate: func(pid int, _ time.Time) error {
			h.terminated = append(h.terminated, pid)
			for _, snap := range h.agents.snapshots {
				for _, s := range snap {
					if s.PID == pid {
						h.agents.markDown(s.SessionID, true)
					}
				}
			}
			if pid != 0 && pid == h.hostPID {
				h.hostDied()
			}
			return nil
		},
		Exec: func(binary string, args ...string) error {
			h.execs = append(h.execs, append([]string{binary}, args...))
			return nil
		},
		Interrupt: func(ctx context.Context) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(ctx)
			h.interrupt = cancel
			return ctx, cancel
		},
		Chdir: func(dir string) error {
			h.chdirs = append(h.chdirs, dir)
			return nil
		},
		Transcripts: transcripts.Store{Dir: filepath.Join(h.root, "projects"), Moves: filepath.Join(h.root, "state", "moves.json")},
		Teams:       teams.Store{Teams: filepath.Join(h.root, "teams"), Tasks: filepath.Join(h.root, "tasks")},
	}
	return h
}

// host makes s the session running claudes. Stopping, respawning, or
// terminating it cuts all later output.
func (h *harness) host(s claude.Session) {
	h.app.HostID = s.SessionID
	h.agents.hostJob = s.JobID
	h.agents.onHost = h.hostDied
	if s.Interactive() {
		h.hostPID = s.PID
	}
}

func (h *harness) hostDied() { h.out.cut, h.err.cut = true, true }

// world replaces the fake's snapshots and forgets which sessions it stopped.
func (h *harness) world(snapshots ...[]claude.Session) {
	h.agents.snapshots, h.agents.down, h.agents.calls = snapshots, nil, nil
}

func (h *harness) run(args ...string) int { return h.app.Run(args) }

func (h *harness) folder(name string) string {
	dir := filepath.Join(h.root, "work", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	return dir
}

func jobID(name string) string { return fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(name))) }

// sessionID differs from the job id, as it does in real use, so code that
// passes one where the other belongs fails.
func sessionID(name string) string {
	return fmt.Sprintf("%08x-1111-2222-3333-444455556666", crc32.ChecksumIEEE([]byte("session/"+name)))
}

func (h *harness) bg(name, status string) claude.Session {
	return claude.Session{
		JobID: jobID(name), SessionID: sessionID(name), Name: name, Cwd: h.folder(name),
		Kind: "background", State: stateFor(status), Status: status, PID: 1000 + len(name),
		StartedAt: h.now.Add(-time.Hour).UnixMilli(),
	}
}

// stateFor is the state claude agents shows with a status: a session in a
// turn is working, and one between turns is done.
func stateFor(status string) string {
	if status == "busy" || status == "waiting" || status == "shell" {
		return "working"
	}
	return "done"
}

func (h *harness) tty(name, status string) claude.Session {
	s := h.bg(name, status)
	s.JobID, s.Kind, s.State = "", "interactive", ""
	return s
}

func stopped(s claude.Session) claude.Session {
	s.PID, s.Status, s.State = 0, "", "stopped"
	return s
}

func (h *harness) pin(t *testing.T, sessions ...claude.Session) {
	t.Helper()
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.JobID)
	}
	if _, err := h.app.Jobs.SetPinned(ids, true); err != nil {
		t.Fatal(err)
	}
}

func assertCalls(t *testing.T, h *harness, want ...string) {
	t.Helper()
	if diff := cmp.Diff(want, h.agents.calls); diff != "" {
		t.Fatalf("calls (-want +got):\n%s", diff)
	}
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("output does not contain %q:\n%s", want, text)
	}
}

func TestPassThrough(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"claude", "agents"}},
		{[]string{"--cwd", ".", "--model", "opus"}, []string{"claude", "agents", "--cwd", ".", "--model", "opus"}},
		{[]string{"--"}, []string{"claude", "agents"}},
		{[]string{"--", "ls", "--json"}, []string{"claude", "agents", "ls", "--json"}},
		{[]string{"--", "--", "x"}, []string{"claude", "agents", "--", "x"}},
	} {
		h := newHarness(t)
		h.ok(t, tc.args...)
		if len(h.execs) != 1 || !slices.Equal(h.execs[0], tc.want) {
			t.Fatalf("%v: execs %q", tc.args, h.execs)
		}
	}
}

func TestOwnCommandsAndHelpDoNotPassThrough(t *testing.T) {
	for _, args := range [][]string{{"ls"}, {"list"}, {"--help"}, {"help"}, {"--version"}} {
		h := newHarness(t, nil)
		h.ok(t, args...)
		if len(h.execs) != 0 {
			t.Fatalf("%v passed through", args)
		}
	}
}

func TestLsPutsPinnedFirstAndMarksThem(t *testing.T) {
	h := newHarness(t)
	idle, busy, pinned := h.bg("idle-one", "idle"), h.bg("busy-one", "busy"), h.bg("pinned-one", "idle")
	h.agents.snapshots = [][]claude.Session{{idle, busy, pinned}}
	h.pin(t, pinned)

	h.ok(t, "ls")

	lines := strings.Split(h.out.String(), "\n")
	if !strings.HasPrefix(lines[0], "PINNED ") || !strings.HasPrefix(lines[1], "yes ") || !strings.Contains(lines[1], "pinned-one") {
		t.Fatalf("pinned row is not first and marked:\n%s", h.out)
	}
	if !strings.Contains(lines[2], "busy-one") {
		t.Fatalf("busy row is not second:\n%s", h.out)
	}
	if !strings.HasPrefix(lines[2], "- ") || strings.Contains(h.out.String(), "* pinned") {
		t.Fatalf("unpinned row or footer wrong:\n%s", h.out)
	}
}

func TestLsMarksSessionsBehindTheInstalledVersion(t *testing.T) {
	h := newHarness(t)
	old := h.bg("old-one", "idle")
	h.agents.snapshots = [][]claude.Session{{old}}
	writeState(t, h, old.JobID, "2.1.322")

	h.ok(t, "ls")

	assertContains(t, h.out.String(), "2.1.322 (old)")
}

func writeState(t *testing.T, h *harness, jobID, cliVersion string) {
	t.Helper()
	dir := filepath.Join(h.app.Jobs.Dir, jobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"cliVersion": %q}`, cliVersion)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
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

func TestRestartInteractiveStopsAndPrintsTheCommand(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "busy")
	h.agents.snapshots = [][]claude.Session{{term}}

	h.ok(t, "restart", "--interactive", "--ping")

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

func TestPauseSavesTheListBeforeStopping(t *testing.T) {
	h := newHarness(t)
	a, busy, p := h.bg("alpha", "idle"), h.bg("busy", "busy"), h.bg("pinned", "idle")
	h.agents.snapshots = [][]claude.Session{{a, busy, p}}
	h.pin(t, p)
	h.agents.beforeStop = func(string) {
		if entries, _ := h.app.Park.Load(); len(entries) != 2 {
			t.Errorf("stopped with %d session(s) parked", len(entries))
		}
	}

	h.ok(t, "pause")

	assertCalls(t, h, "stop "+a.JobID, "stop "+busy.JobID)
	entries, err := h.app.Park.Load()
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries %v, err %v", entries, err)
	}
	if entries[0].WasBusy || !entries[1].WasBusy {
		t.Fatalf("busy flags wrong: %+v", entries)
	}
	assertContains(t, h.out.String(), "Bring them back with: claudes resume")
}

func TestPauseDropsASessionThatWouldNotStop(t *testing.T) {
	h := newHarness(t)
	bad, good := h.bg("bad", "idle"), h.bg("good", "idle")
	h.agents.snapshots = [][]claude.Session{{bad, good}}
	h.agents.failures = map[string]string{"stop " + bad.JobID: "boom"}

	h.fails(t, "pause")

	entries, _ := h.app.Park.Load()
	if len(entries) != 1 || entries[0].Name != "good" {
		t.Fatalf("entries %+v", entries)
	}
}

func TestResumeBringsParkedSessionsBackAndClearsTheFile(t *testing.T) {
	h := newHarness(t)
	a, busy := h.bg("alpha", "idle"), h.bg("busy", "busy")
	h.agents.snapshots = [][]claude.Session{{a, busy}, {}}
	h.ok(t, "pause")
	h.agents.calls = nil

	h.ok(t, "resume", "--ping")

	assertCalls(t, h,
		"resume "+a.SessionID+" in alpha prompt=",
		"resume "+busy.SessionID+" in busy prompt="+defaultPing,
	)
	if _, err := os.Stat(h.app.Park.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("park file still there: %v", err)
	}
}

func TestResumeKeepsFailuresAndSkipsRunningSessions(t *testing.T) {
	h := newHarness(t)
	a, b, c := h.bg("alpha", "idle"), h.bg("bravo", "idle"), h.bg("charlie", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b, c}}
	h.ok(t, "pause")
	h.world([]claude.Session{c})
	h.agents.failures = map[string]string{"resume " + b.SessionID + " in bravo prompt=": "folder is gone"}

	h.fails(t, "resume")

	entries, _ := h.app.Park.Load()
	if len(entries) != 1 || entries[0].Name != "bravo" {
		t.Fatalf("entries %+v", entries)
	}
	assertContains(t, h.out.String(), "Already running, dropped from the park list: charlie")
}

func TestResumeSkipKeepsTheEntryParked(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("bravo", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}, {}}
	h.ok(t, "pause")
	h.agents.calls = nil

	h.ok(t, "resume", "--skip", "bravo")

	assertCalls(t, h, "resume "+a.SessionID+" in alpha prompt=")
	entries, _ := h.app.Park.Load()
	if len(entries) != 1 || entries[0].Name != "bravo" {
		t.Fatalf("entries %+v", entries)
	}
}

func TestResumeRecoversAStoppedSessionByName(t *testing.T) {
	h := newHarness(t)
	db := stopped(h.bg("db-upgrade-plan", ""))
	h.agents.snapshots = [][]claude.Session{{}}
	h.agents.finished = []claude.Session{db}

	h.ok(t, "resume", "db", "--ping")

	assertCalls(t, h, "resume "+db.SessionID+" in db-upgrade-plan prompt="+defaultPing)
}

func TestNothingParked(t *testing.T) {
	h := newHarness(t, nil)
	h.ok(t, "resume")
	assertContains(t, h.out.String(), "Nothing is parked.")
}

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

func TestLogsAttachAndWhere(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}

	h.ok(t, "logs", "alp")
	h.ok(t, "attach", "alpha")
	h.ok(t, "where", "alpha")

	want := [][]string{{"claude", "logs", a.JobID}, {"claude", "attach", a.JobID}}
	if len(h.execs) != 2 || !slices.Equal(h.execs[0], want[0]) || !slices.Equal(h.execs[1], want[1]) {
		t.Fatalf("execs %q", h.execs)
	}
	assertContains(t, h.out.String(), a.Cwd)
}

func TestGCRemovesOldFinishedSessionsExceptPinnedOnes(t *testing.T) {
	h := newHarness(t)
	old, recent, oldPinned, live := stopped(h.bg("old", "")), stopped(h.bg("recent", "")), stopped(h.bg("old-pinned", "")), h.bg("live", "idle")
	old.StartedAt = h.now.Add(-10 * 24 * time.Hour).UnixMilli()
	oldPinned.StartedAt = old.StartedAt
	h.agents.snapshots = [][]claude.Session{{live}}
	h.agents.finished = []claude.Session{old, recent, oldPinned}
	h.pin(t, oldPinned)

	h.ok(t, "gc", "--dry-run")
	assertCalls(t, h)

	h.ok(t, "gc")
	assertCalls(t, h, "rm "+old.JobID)
}

func TestGCOlderThanAcceptsDays(t *testing.T) {
	var d days
	if err := d.Set("3d"); err != nil || d.d != 72*time.Hour {
		t.Fatalf("3d: %v %v", d.d, err)
	}
	if err := d.Set("12h"); err != nil || d.d != 12*time.Hour {
		t.Fatalf("12h: %v %v", d.d, err)
	}
	if err := d.Set("soon"); err == nil {
		t.Fatal("accepted soon")
	}
}

func TestAMissingHomeFolderStopsEveryCommand(t *testing.T) {
	for _, name := range []string{"HOME", "USERPROFILE", "CLAUDE_CONFIG_DIR", "XDG_STATE_HOME"} {
		t.Setenv(name, "")
	}
	app := newApp()
	var errOut bytes.Buffer
	app.Err = &errOut
	app.Agents = &fakeAgents{}
	if code := app.Run([]string{"restart"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	assertContains(t, errOut.String(), "cannot find")
}

// ok runs claudes and fails the test unless it exits 0.
func (h *harness) ok(t *testing.T, args ...string) {
	t.Helper()
	if code := h.run(args...); code != 0 {
		t.Fatalf("claudes %q: exit %d\n%s", args, code, h.err)
	}
}

// fails runs claudes and fails the test unless it exits with a failure code,
// 1 to 5.
func (h *harness) fails(t *testing.T, args ...string) {
	t.Helper()
	if code := h.run(args...); code < exitFailed || code > exitBusy {
		t.Fatalf("claudes %q: exit %d, want 1 to 5\n%s%s", args, code, h.out, h.err)
	}
}

// exits runs claudes and fails the test unless it exits with want.
func (h *harness) exits(t *testing.T, want int, args ...string) {
	t.Helper()
	if code := h.run(args...); code != want {
		t.Fatalf("claudes %q: exit %d, want %d\n%s%s", args, code, want, h.out, h.err)
	}
}

func TestExitCodesSayWhatHappened(t *testing.T) {
	h := newHarness(t)
	one, two := h.bg("review-one", "idle"), h.bg("review-two", "idle")
	bad := h.bg("bad", "idle")
	h.agents.snapshots = [][]claude.Session{{one, two, bad}}

	h.exits(t, exitNoMatch, "stop", "nothing-like-it")
	h.exits(t, exitNoMatch, "stop", "review")

	h.agents.failures = map[string]string{"stop " + bad.JobID: "boom"}
	h.exits(t, exitSomeFailed, "stop", "bad")

	odd := h.bg("odd", "idle")
	odd.Kind = "daemon"
	h.agents.snapshots = [][]claude.Session{{odd}}
	h.exits(t, exitRefused, "stop", "odd")
}

func TestOneClaudesChangesSessionsAtATime(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	release, err := safefile.Lock{Path: h.app.LockPath, Stale: time.Minute}.Acquire()
	if err != nil {
		t.Fatal(err)
	}

	h.exits(t, exitBusy, "stop", "api")
	assertContains(t, h.err.String(), "another claudes is changing sessions")
	h.ok(t, "restart", "--dry-run", "api")
	h.ok(t, "ls")
	if len(h.agents.calls) != 0 {
		t.Fatalf("calls %q while another claudes held the lock", h.agents.calls)
	}

	release()
	h.ok(t, "stop", "api")
	if _, err := os.Stat(h.app.LockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lock outlived the run: %v", err)
	}
}

func TestTheLockIsFreeBeforeTheHostStops(t *testing.T) {
	h := newHarness(t)
	host := h.bg("host", "idle")
	h.agents.snapshots = [][]claude.Session{{host}}
	h.host(host)
	var heldAtStop bool
	h.agents.beforeStop = func(string) {
		_, err := os.Stat(h.app.LockPath)
		heldAtStop = err == nil
	}

	h.ok(t, "stop", "host")
	if heldAtStop {
		t.Fatal("the lock was still held when the host stopped, so it would outlive claudes")
	}
}

func TestVerboseLogsTheStepsToStderr(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	h.ok(t, "ls")
	if strings.Contains(h.err.String(), "level=DEBUG") {
		t.Fatalf("logged without -v:\n%s", h.err)
	}
	h.ok(t, "stop", "-v", "api")
	assertContains(t, h.err.String(), "level=DEBUG msg=\"took the run lock\"")
	assertContains(t, h.out.String(), "stopped")
}

func TestClaudesLogSetsTheLevel(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	t.Setenv("CLAUDES_LOG", "debug")
	h.ok(t, "ls")
	assertContains(t, h.err.String(), "level=DEBUG")

	h.err.Reset()
	t.Setenv("CLAUDES_LOG", "loud")
	h.ok(t, "ls")
	assertContains(t, h.err.String(), "CLAUDES_LOG must be debug, info, warn, or error")
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

func TestPingTakesOneSessionAndAMessage(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}

	h.fails(t, "ping", "alpha", "beta", "sup")
	h.ok(t, "ping", "alpha", "sup dude")
	assertCalls(t, h, "stop "+a.JobID, "resume "+a.SessionID+" in alpha prompt=sup dude")
}

func TestAWorkingSessionWithALaggingStatusIsPausedAsBusy(t *testing.T) {
	h := newHarness(t)
	s := h.bg("counting", "idle")
	s.State = "working"
	h.agents.snapshots = [][]claude.Session{{s}}

	h.ok(t, "pause", "counting")
	assertContains(t, h.out.String(), "busy")
	entries, err := h.app.Park.Load()
	if err != nil || len(entries) != 1 || !entries[0].WasBusy {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
	h.agents.calls = nil
	h.ok(t, "resume", "--ping")
	assertCalls(t, h, "resume "+s.SessionID+" in counting prompt="+defaultPing)
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

func (h *harness) logLines(t *testing.T, s claude.Session, lines ...string) string {
	t.Helper()
	path := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(s.Cwd), s.SessionID+".jsonl")
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	return path
}

func TestLogsTailPrintsTheLastEvents(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s,
		`{"type":"user","message":{"content":"one"}}`,
		`{"type":"user","message":{"content":"two"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"three"}]}}`,
	)

	h.ok(t, "logs", "api", "--tail", "2")
	out := h.out.String()
	if strings.Contains(out, "one") || !strings.Contains(out, "you    two") || !strings.Contains(out, "claude three") {
		t.Fatalf("output:\n%s", out)
	}
	if len(h.execs) != 0 {
		t.Fatalf("handed off to claude logs: %q", h.execs)
	}

	h.out.Reset()
	h.ok(t, "logs", "api", "--tail", "1", "-o", "json")
	if got := strings.TrimSpace(h.out.String()); got != `{"type":"assistant","message":{"content":[{"type":"text","text":"three"}]}}` {
		t.Fatalf("json %q", got)
	}
}

func TestLogsFollowPrintsNewEventsUntilCtrlC(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	path := h.logLines(t, s, `{"type":"user","message":{"content":"old"}}`)
	sleeps := 0
	h.app.Sleep = func(ctx context.Context, _ time.Duration) error {
		sleeps++
		switch sleeps {
		case 1:
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprint(f, `{"type":"assistant","message":{"content":[{"type":"text","text":"new"}]}}`+"\n"+`{"type":"user","message":{"cont`)
			f.Close()
		case 2:
			h.interrupt()
			return ctx.Err()
		}
		return nil
	}

	h.ok(t, "logs", "-f", "api", "--tail", "0")
	out := h.out.String()
	if strings.Contains(out, "old") || !strings.Contains(out, "claude new") {
		t.Fatalf("output:\n%s", out)
	}
	if strings.Contains(h.err.String(), "interrupted") {
		t.Fatalf("Ctrl-C was reported as an interruption:\n%s", h.err)
	}
}

func TestLogsWithoutFlagsHandsOffToClaude(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}

	h.ok(t, "logs", "api")
	if len(h.execs) != 1 || !slices.Equal(h.execs[0], []string{"claude", "logs", s.JobID}) {
		t.Fatalf("execs %q", h.execs)
	}
}

func TestListingsShowNoMatchAsAnEmptyList(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	h.ok(t, "ls", "tour*")
	assertContains(t, h.out.String(), "No sessions.")
	h.out.Reset()
	h.ok(t, "ls", "tour*", "-o", "json")
	if got := strings.TrimSpace(h.out.String()); got != "[]" {
		t.Fatalf("json %q", got)
	}
	h.ok(t, "session", "ls", "tour*")
	h.exits(t, exitNoMatch, "where", "tour*")
}

// team writes the config of the team s leads, with these teammates at work.
func (h *harness) team(t *testing.T, s claude.Session, teammates ...string) {
	t.Helper()
	members := []map[string]string{{"name": "team-lead", "agentType": "team-lead", "backendType": "in-process", "cwd": s.Cwd}}
	for _, n := range teammates {
		members = append(members, map[string]string{"name": n, "agentType": "general-purpose", "backendType": "in-process", "cwd": s.Cwd})
	}
	name := teams.Name(s.SessionID)
	data, err := json.Marshal(map[string]any{"name": name, "leadSessionId": s.SessionID, "members": members})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.app.Teams.Teams, name, "config.json"), string(data))
}

func TestBulkCommandsLeaveALeadWithTeammatesAlone(t *testing.T) {
	h := newHarness(t)
	lead, other := h.bg("lead", "idle"), h.bg("other", "idle")
	h.agents.snapshots = [][]claude.Session{{lead, other}}
	h.team(t, lead, "writer", "tester")

	h.ok(t, "restart", "--dry-run")
	out := h.out.String()
	assertContains(t, out, "Leading an agent team, not restarted: lead")
	if strings.Contains(out, "  "+lead.JobID+"  ") {
		t.Fatalf("the lead is in the plan:\n%s", out)
	}
	h.out.Reset()
	h.ok(t, "pause", "--dry-run")
	assertContains(t, h.out.String(), "Leading an agent team, not paused: lead")
}

func TestANamedLeadIsRestartedAndPingedWithAWarning(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead, "writer")

	h.ok(t, "restart", "--ping", "lead")
	assertContains(t, h.out.String(), "leads 1 teammate(s): writer, which stop with it")
	assertCalls(t, h, "stop "+lead.JobID, "resume "+lead.SessionID+" in lead prompt="+defaultPing)

	h.err.Reset()
	h.ok(t, "stop", "lead")
	assertContains(t, h.err.String(), "lead leads 1 teammate(s): writer")
}

func TestALeadWhoseTeammatesShutDownIsAnOrdinarySession(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead)

	h.ok(t, "restart", "--dry-run")
	assertContains(t, h.out.String(), "Restarting 1 session(s)")
}

func TestLsShowsWhichSessionsLeadATeam(t *testing.T) {
	h := newHarness(t)
	lead, solo := h.bg("lead", "idle"), h.bg("solo", "idle")
	h.agents.snapshots = [][]claude.Session{{lead, solo}}
	h.team(t, lead, "writer", "tester")

	h.ok(t, "ls")
	assertContains(t, h.out.String(), "lead+2")
	h.out.Reset()
	h.ok(t, "ls", "lead", "-o", "json")
	var rows []struct{ Teammates []string }
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 1 || !slices.Equal(rows[0].Teammates, []string{"writer", "tester"}) {
		t.Fatalf("rows %+v, err %v\n%s", rows, err, h.out)
	}
}

func TestTeamLsShowsTeamsAtWork(t *testing.T) {
	h := newHarness(t)
	lead, solo := h.bg("lead", "busy"), h.bg("solo", "idle")
	gone := stopped(h.bg("gone", ""))
	h.agents.snapshots = [][]claude.Session{{lead, solo}}
	h.agents.finished = []claude.Session{gone}
	h.team(t, lead, "writer", "tester")
	h.team(t, solo)
	h.team(t, gone)
	tasks := filepath.Join(h.app.Teams.Tasks, teams.Name(lead.SessionID))
	writeFile(t, filepath.Join(tasks, "1.json"), `{"id":"1","subject":"write","status":"completed","owner":"writer"}`)
	writeFile(t, filepath.Join(tasks, "2.json"), `{"id":"2","subject":"test","status":"in_progress","owner":"tester"}`)

	h.ok(t, "team", "ls")
	out := h.out.String()
	assertContains(t, out, "writer, tester")
	assertContains(t, out, "1/2 done, working: tester")
	if strings.Contains(out, "solo") || strings.Contains(out, "gone") {
		t.Fatalf("a team with no teammates is listed:\n%s", out)
	}

	h.out.Reset()
	h.ok(t, "team", "ls", "-a")
	assertContains(t, h.out.String(), "solo")
	assertContains(t, h.out.String(), "gone (not running)")

	h.out.Reset()
	h.ok(t, "team", "ls", "lead", "-o", "json")
	var rows []teamRow
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 1 || len(rows[0].Tasks) != 2 || !rows[0].Running {
		t.Fatalf("rows %+v, err %v", rows, err)
	}
}

func TestLogsShowsATeammateByLeadAndName(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "busy")
	h.agents.snapshots = [][]claude.Session{{lead}}
	path := h.logLines(t, lead, `{"type":"user","message":{"content":"go"}}`)
	writeFile(t, filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-atester-abcd.jsonl"),
		`{"type":"assistant","message":{"content":[{"type":"text","text":"tests pass"}]}}`+"\n")
	writeFile(t, filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-atester-abcd.meta.json"), `{"teamName":"session-x","taskKind":"in_process_teammate"}`)

	h.ok(t, "logs", "lead/tester")
	assertContains(t, h.out.String(), "claude tests pass")
	if len(h.execs) != 0 {
		t.Fatalf("handed off to claude logs: %q", h.execs)
	}
	h.fails(t, "logs", "lead/nobody")
	assertContains(t, h.err.String(), "teammates: tester")
}

func TestGCTeamsRemovesOnlyOldFoldersOfSessionsThatAreGone(t *testing.T) {
	h := newHarness(t)
	live := h.bg("live", "idle")
	h.agents.snapshots = [][]claude.Session{{live}}
	old := h.now.Add(-30 * 24 * time.Hour)
	for _, p := range []string{
		filepath.Join(h.app.Teams.Teams, "session-aaaaaaaa", "config.json"),
		filepath.Join(h.app.Teams.Tasks, "session-aaaaaaaa", "1.json"),
		filepath.Join(h.app.Teams.Tasks, teams.Name(live.SessionID), "1.json"),
	} {
		writeFile(t, p, "{}")
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	h.ok(t, "gc", "--teams", "--dry-run")
	assertContains(t, h.out.String(), "Removing 2 team folder(s) and task list(s)")
	if _, err := os.Stat(filepath.Join(h.app.Teams.Teams, "session-aaaaaaaa")); err != nil {
		t.Fatalf("a dry run removed a folder: %v", err)
	}

	h.ok(t, "gc", "--teams")
	for _, gone := range []string{filepath.Join(h.app.Teams.Teams, "session-aaaaaaaa"), filepath.Join(h.app.Teams.Tasks, "session-aaaaaaaa")} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s is still there: %v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(h.app.Teams.Tasks, teams.Name(live.SessionID))); err != nil {
		t.Fatalf("a running session's task list went: %v", err)
	}
	h.fails(t, "gc", "--teams", "live")
}
