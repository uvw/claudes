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

	"github.com/uvw/claudes/internal/accounts"
	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/jobs"
	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/teams"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/trash"
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
	// jobsDir is the jobs directory, where rm deletes a job's files.
	jobsDir string
	// others stand in for claude run with another account's
	// CLAUDE_CONFIG_DIR. A call with a ConfigDir that is neither the zero
	// value nor one of them reaches an account that knows no session.
	others map[claude.ConfigDir]*fakeAgents
}

// in is the fake that answers a call made with dir.
func (f *fakeAgents) in(dir claude.ConfigDir) *fakeAgents {
	if dir.Own() {
		return f
	}
	if o, ok := f.others[dir]; ok {
		return o
	}
	return &fakeAgents{}
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

func (f *fakeAgents) Sessions(ctx context.Context, dir claude.ConfigDir, all bool) ([]claude.Session, error) {
	if g := f.in(dir); g != f {
		sessions, err := g.Sessions(ctx, claude.ConfigDir{}, all)
		for i := range sessions {
			sessions[i].ConfigDir = dir
		}
		return sessions, err
	}
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

func (f *fakeAgents) Respawn(ctx context.Context, dir claude.ConfigDir, jobID string) error {
	if g := f.in(dir); g != f {
		return g.Respawn(ctx, claude.ConfigDir{}, jobID)
	}
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

func (f *fakeAgents) Stop(ctx context.Context, dir claude.ConfigDir, jobID string) error {
	if g := f.in(dir); g != f {
		return g.Stop(ctx, claude.ConfigDir{}, jobID)
	}
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

// Remove drops a finished job from the list and deletes its directory, as
// claude rm does.
func (f *fakeAgents) Remove(ctx context.Context, dir claude.ConfigDir, jobID string) error {
	if g := f.in(dir); g != f {
		return g.Remove(ctx, claude.ConfigDir{}, jobID)
	}
	err := f.job("rm", jobID)
	if err == nil {
		f.finished = slices.DeleteFunc(slices.Clone(f.finished), func(s claude.Session) bool { return s.JobID == jobID })
		if f.jobsDir != "" && jobIDPattern.MatchString(jobID) {
			err = os.RemoveAll(filepath.Join(f.jobsDir, jobID))
		}
	}
	return err
}

func (f *fakeAgents) Resume(ctx context.Context, dir claude.ConfigDir, sessionID, cwd, prompt string, job claude.NewJob) (string, error) {
	if g := f.in(dir); g != f {
		return g.Resume(ctx, claude.ConfigDir{}, sessionID, cwd, prompt, job)
	}
	call := sessionID + " in " + filepath.Base(cwd) + " prompt=" + prompt
	if job.Name != "" {
		call += " name=" + job.Name
	}
	if len(job.Options) > 0 {
		call += " options=" + strings.Join(job.Options, " ")
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
	// terminatedSince is the session start time each Terminate was given.
	terminatedSince []time.Time
	execs           [][]string
	// execConfigs is the ConfigDir each exec ran with.
	execConfigs []claude.ConfigDir
	chdirs      []string
	trashed     []string
	sleeps      []time.Duration
	hostPID     int
	now         time.Time
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
	// Resolved as Claude Code records a directory: macOS reaches the temporary
	// directory through the /var link, and Windows may name it in 8.3 form.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Claude Code creates the jobs directory; a missing one means the wrong directory.
	if err := os.MkdirAll(filepath.Join(root, "jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &harness{agents: &fakeAgents{snapshots: snapshots, jobsDir: filepath.Join(root, "jobs")}, out: &cutWriter{}, err: &cutWriter{}, root: root, now: start}
	own := &account{
		Account: accounts.Account{Name: "~/.claude", Dir: h.root, ConfigFile: filepath.Join(h.root, ".claude.json"), From: []string{accounts.FromHome}},
		Jobs:    jobs.Store{Dir: filepath.Join(h.root, "jobs"), LockAttempts: 3, LockDelay: time.Millisecond},
		MCP:     mcp.Store{ConfigFile: filepath.Join(h.root, ".claude.json"), Home: h.root},
		Teams:   teams.Store{Teams: filepath.Join(h.root, "teams"), Tasks: filepath.Join(h.root, "tasks")},
	}
	h.app = &App{
		Agents:   h.agents,
		Accounts: []*account{own},
		Found:    accounts.Found{Home: h.root, Accounts: []accounts.Account{own.Account}},
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
		Terminate: func(pid int, sessionStarted, _ time.Time) error {
			h.terminated = append(h.terminated, pid)
			h.terminatedSince = append(h.terminatedSince, sessionStarted)
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
		Exec: func(config claude.ConfigDir, binary string, args ...string) error {
			h.execs = append(h.execs, append([]string{binary}, args...))
			h.execConfigs = append(h.execConfigs, config)
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
		// Trash moves into a directory of the test's own, under a numbered
		// name, as the macOS Trash does.
		Trash: func(path string) (string, error) {
			h.trashed = append(h.trashed, path)
			to := filepath.Join(h.root, "Trash", fmt.Sprintf("%d %s", len(h.trashed), filepath.Base(path)))
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return "", err
			}
			return to, os.Rename(path, to)
		},
		Trashed:     trash.Manifest{Path: filepath.Join(h.root, "state", "trashed.json")},
		Transcripts: transcripts.Store{Dir: filepath.Join(h.root, "projects"), Home: h.root, Moves: filepath.Join(h.root, "state", "moves.json")},
		Executable:  func() (string, error) { return filepath.Join(h.root, "bin", "claudes"), nil },
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
	if _, err := h.app.Accounts[0].Jobs.SetPinned(ids, true); err != nil {
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

func writeState(t *testing.T, h *harness, jobID, cliVersion string) {
	t.Helper()
	dir := filepath.Join(h.app.Accounts[0].Jobs.Dir, jobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"cliVersion": %q}`, cliVersion)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
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
	writeFile(t, filepath.Join(h.app.Accounts[0].Teams.Teams, name, "config.json"), string(data))
}

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

func (h *harness) env(vars ...string) {
	h.app.Environ = func() []string { return vars }
}

func assertPlain(t *testing.T, what, text string) {
	t.Helper()
	if strings.Contains(text, "\x1b") {
		t.Fatalf("%s has colors:\n%q", what, text)
	}
}

// lsWorld has a session of each kind that ls colors.
func lsWorld(t *testing.T, h *harness) {
	t.Helper()
	pinned, old := h.bg("pinned-one", "busy"), h.bg("old-one", "idle")
	h.agents.snapshots = [][]claude.Session{{pinned, h.bg("asking", "waiting"), h.tty("prompt", "shell"), old}}
	h.pin(t, pinned)
	writeState(t, h, old.JobID, "2.1.322")
}

// transcript writes a transcript for a session named name that ran in cwd and
// was last active ago before now. It returns the session id.
func (h *harness) transcript(t *testing.T, name, cwd string, ago time.Duration) string {
	t.Helper()
	id := sessionID(name + cwd + ago.String())
	return h.transcriptAs(t, id, name, cwd, ago)
}

func (h *harness) transcriptAs(t *testing.T, id, name, cwd string, ago time.Duration) string {
	t.Helper()
	var b strings.Builder
	for _, r := range []map[string]any{
		{"type": "user", "sessionId": id, "cwd": cwd, "gitBranch": "main", "timestamp": h.now.Add(-ago - time.Hour).Format(time.RFC3339), "version": "2.1.325"},
		{"type": "custom-title", "customTitle": name, "sessionId": id},
		{"type": "last-prompt", "lastPrompt": "carry on\nwith the plan", "sessionId": id},
	} {
		if name == "" && r["type"] == "custom-title" {
			continue
		}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	path := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(cwd), id+".jsonl")
	writeFile(t, path, b.String())
	at := h.now.Add(-ago)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	return id
}

func jsonQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// stoppedJob is a stopped background session with a transcript and a
// state.json that holds options.
//
//nolint:unparam // Named at the call, as bg is, so the test shows the name.
func (h *harness) stoppedJob(t *testing.T, name string, options ...string) claude.Session {
	t.Helper()
	s := stopped(h.bg(name, "idle"))
	h.transcriptAs(t, s.SessionID, name, s.Cwd, time.Hour)
	flags, _ := json.Marshal(options)
	writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, s.JobID, "state.json"),
		fmt.Sprintf(`{"sessionId":%q,"cwd":%q,"respawnFlags":%s}`, s.SessionID, s.Cwd, flags))
	h.agents.finished = append(h.agents.finished, s)
	return s
}

// withConfig points h at a config.yaml in its temporary directory, loaded the way
// newApp loads it.
func (h *harness) withConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(h.root, "config", "config.yaml")
	if body != "" {
		writeFile(t, path, body)
	}
	h.app.ConfigPath = path
	h.app.Config, h.app.configErr = config.Load(path)
	return path
}

// account adds an account named name, whose sessions a fake of its own lists,
// reached with its directory as CLAUDE_CONFIG_DIR. A call for one of its
// sessions made without that directory reaches the wrong fake, which knows no
// such job.
//
//nolint:unparam // Named at the call, as bg is, so the test shows the name.
func (h *harness) account(name string) (*account, *fakeAgents) {
	dir := filepath.Join(h.root, "accounts", name)
	if err := os.MkdirAll(filepath.Join(dir, "jobs"), 0o755); err != nil {
		panic(err)
	}
	a := &account{
		Account: accounts.Account{Name: name, Dir: dir, ConfigFile: filepath.Join(dir, ".claude.json"), ConfigDir: claude.ConfigDir{Path: dir}, From: []string{accounts.FromConfig}},
		Jobs:    jobs.Store{Dir: filepath.Join(dir, "jobs"), LockAttempts: 3, LockDelay: time.Millisecond},
		MCP:     mcp.Store{ConfigFile: filepath.Join(dir, ".claude.json"), Home: dir},
		Teams:   teams.Store{Teams: filepath.Join(dir, "teams"), Tasks: filepath.Join(dir, "tasks")},
	}
	fake := &fakeAgents{jobsDir: a.Jobs.Dir}
	if h.agents.others == nil {
		h.agents.others = map[claude.ConfigDir]*fakeAgents{}
	}
	h.agents.others[a.ConfigDir] = fake
	h.app.Accounts = append(h.app.Accounts, a)
	h.app.Found.Accounts = append(h.app.Found.Accounts, a.Account)
	h.app.Transcripts.Projects = append(h.app.Transcripts.Projects, filepath.Join(dir, "projects"))
	h.app.Transcripts.Homes = append(h.app.Transcripts.Homes, dir)
	return a, fake
}
