package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/transcripts"
)

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

func TestSessionLsListsTranscriptsNewestFirst(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	old := h.transcript(t, "old-plan", work, 72*time.Hour)
	live := h.bg("live", "busy")
	h.transcriptAs(t, live.SessionID, "", other, time.Minute)
	h.agents.snapshots = [][]claude.Session{{live}}

	h.ok(t, "session", "ls")
	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[1], "live") || !strings.Contains(lines[1], " yes ") ||
		!strings.HasSuffix(lines[2], "old-plan") || !strings.Contains(lines[2], "3d") {
		t.Fatalf("table:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "--since", "1d", "-o", "json")
	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["sessionId"] != live.SessionID || rows[0]["running"] != true || rows[0]["id"] != live.SessionID[:8] {
		t.Fatalf("rows %v", rows)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "--in", work)
	if !strings.Contains(h.out.String(), "old-plan") || strings.Contains(h.out.String(), "live") {
		t.Fatalf("--in:\n%s", h.out)
	}
	h.out.Reset()
	h.ok(t, "session", "ls", old[:8])
	if !strings.Contains(h.out.String(), "old-plan") || strings.Contains(h.out.String(), "live") {
		t.Fatalf("by id:\n%s", h.out)
	}
}

func TestSessionAnswersToSAndSessions(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "old-plan", h.folder("work"), 72*time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}

	for _, alias := range []string{"s", "sessions"} {
		h.out.Reset()
		h.ok(t, alias, "ls")
		if !strings.Contains(h.out.String(), "old-plan") {
			t.Fatalf("%s ls:\n%s", alias, h.out)
		}
	}
	if len(h.execs) != 0 {
		t.Fatalf("an alias passed through to claude agents: %q", h.execs)
	}
}

func TestSessionShowAndWherePickTheNewestOfAName(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("before"), 48*time.Hour)
	h.transcript(t, "plan", h.folder("after"), time.Hour)
	h.transcript(t, "planner", h.folder("other"), 2*time.Hour)

	if code := h.run("session", "where", "--one", "plan"); code != 0 || strings.TrimSpace(h.out.String()) != h.folder("after") {
		t.Fatalf("exit %d, %q", code, h.out)
	}
	h.out.Reset()
	h.ok(t, "session", "show", "plan")
	assertContains(t, h.out.String(), h.folder("after"))
	assertContains(t, h.out.String(), "carry on …")
}

func TestSessionWhereOneRefusesSeveralSessions(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "alpha-one", h.folder("a"), time.Hour)
	h.transcript(t, "alpha-two", h.folder("b"), time.Hour)

	h.exits(t, 2, "session", "where", "--one", "alpha")
	assertContains(t, h.err.String(), "matches 2 sessions")
	h.err.Reset()
	h.exits(t, 2, "session", "where", "--one", "alpha-one", "alpha-two")
	assertContains(t, h.err.String(), "2 sessions match")
	h.err.Reset()
	h.ok(t, "session", "where", "alpha")
	assertContains(t, h.out.String(), "alpha-one")
	assertContains(t, h.out.String(), "alpha-two")
}

func TestSessionResumeRunsClaudeInTheSessionsFolder(t *testing.T) {
	h := newHarness(t)
	dir := h.folder("work")
	id := h.transcript(t, "plan", dir, time.Hour)

	h.ok(t, "session", "resume", "plan", "keep going")
	if !slices.Equal(h.chdirs, []string{dir}) || len(h.execs) != 1 ||
		!slices.Equal(h.execs[0], []string{"claude", "--resume", id, "keep going"}) {
		t.Fatalf("chdirs %q, execs %q", h.chdirs, h.execs)
	}

	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+id+" in work prompt= name=plan")
}

func TestSessionResumeRefusesRunningMissingAndOptionLikeInput(t *testing.T) {
	h := newHarness(t)
	live := h.bg("live", "idle")
	h.transcriptAs(t, live.SessionID, "live", h.folder("live"), time.Minute)
	h.agents.snapshots = [][]claude.Session{{live}}
	h.transcript(t, "gone", filepath.Join(h.root, "deleted"), time.Hour)

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"live"}, "would start a copy"},
		{[]string{"gone"}, "which is gone"},
		{[]string{"gone", "--", "--model"}, "read it as an option"},
	} {
		h.err.Reset()
		h.fails(t, append([]string{"session", "resume"}, c.args...)...)
		assertContains(t, h.err.String(), c.want)
	}
	if len(h.execs) != 0 || len(h.agents.calls) != 0 {
		t.Fatalf("execs %q, calls %q", h.execs, h.agents.calls)
	}
}

func TestSessionMvCopiesAndPrintsTheWayBack(t *testing.T) {
	h := newHarness(t)
	from, to := h.folder("from"), h.folder("to")
	id := h.transcript(t, "plan", from, time.Hour)
	src := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(from), id+".jsonl")
	writeFile(t, filepath.Join(strings.TrimSuffix(src, ".jsonl"), "subagents", "a.jsonl"), "{}\n")

	h.ok(t, "session", "mv", "plan", to)
	dest := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(to))
	for _, p := range []string{filepath.Join(dest, id+".jsonl"), filepath.Join(dest, id, "subagents", "a.jsonl"), src} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	assertContains(t, h.out.String(), shell.RemoveCommand([]string{src, strings.TrimSuffix(src, ".jsonl")}))
	assertContains(t, h.out.String(), shell.ResumeCommand(id, to, true))
	if len(h.execs) != 0 {
		t.Fatalf("execs %q", h.execs)
	}

	// The copy's records still name the old directory; claudes' own record of
	// the move points at the new one.
	h.out.Reset()
	h.ok(t, "session", "where", "--one", "plan")
	if got := strings.TrimSpace(h.out.String()); got != to {
		t.Fatalf("where %q after the move, want %q", got, to)
	}
}

func TestSessionMvResumeRunsTheCopy(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	id := h.transcript(t, "plan", h.folder("from"), time.Hour)

	h.ok(t, "session", "mv", "--resume", "plan", to)
	if !slices.Equal(h.chdirs, []string{to}) || len(h.execs) != 1 || !slices.Equal(h.execs[0], []string{"claude", "--resume", id}) {
		t.Fatalf("chdirs %q, execs %q", h.chdirs, h.execs)
	}
}

func TestSessionMvRefusesARunningSessionButNotTheHost(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	other, host := h.bg("other", "idle"), h.bg("host", "busy")
	h.transcriptAs(t, other.SessionID, "other", h.folder("o"), time.Minute)
	h.transcriptAs(t, host.SessionID, "host", h.folder("h"), time.Minute)
	h.agents.snapshots = [][]claude.Session{{other, host}}
	h.host(host)

	h.fails(t, "session", "mv", "other", to)
	assertContains(t, h.err.String(), "Stop it first")
	h.fails(t, "session", "mv", "--resume", "host", to)
	h.ok(t, "session", "mv", "host", to)
	assertContains(t, h.out.String(), "Exit it now")
}

func TestSessionMvRefusesAMissingFolderAndItsOwnFolder(t *testing.T) {
	h := newHarness(t)
	from := h.folder("from")
	h.transcript(t, "plan", from, time.Hour)

	h.fails(t, "session", "mv", "plan", filepath.Join(h.root, "nope"))
	assertContains(t, h.err.String(), "does not exist")
	h.fails(t, "session", "mv", "plan", from)
	assertContains(t, h.err.String(), "already lives under")
}

func TestSessionMvChecksFolderNaming(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	cwd := h.folder("from")
	id := sessionID("odd")
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, "renamed", id+".jsonl"),
		`{"type":"user","sessionId":"`+id+`","cwd":`+jsonQuote(cwd)+`}`+"\n")

	h.fails(t, "session", "mv", id[:8], to)
	assertContains(t, h.err.String(), "--force")
	h.ok(t, "session", "mv", "--force", id[:8], to)
}

func jsonQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func TestInitAndCd(t *testing.T) {
	h := newHarness(t)
	for shell, want := range map[string]string{
		"zsh":  "command claudes session where --one",
		"fish": "and cd -- $dir",
		"pwsh": "Set-Location -LiteralPath $dir",
	} {
		h.out.Reset()
		h.ok(t, "init", shell)
		assertContains(t, h.out.String(), want)
	}
	h.fails(t, "init", "tcsh")
	h.fails(t, "cd", "plan")
	assertContains(t, h.err.String(), "claudes init pwsh")
}

func TestSessionMoveIsMv(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("from"), time.Hour)
	h.ok(t, "session", "move", "plan", h.folder("to"))
	assertContains(t, h.out.String(), "copied plan")
}

func TestSessionResumeAndMvRefuseWhenTheyCannotTellWhatRuns(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	h.transcript(t, "plan", h.folder("from"), time.Hour)
	h.agents.failures = map[string]string{"list": "claude is gone"}

	h.fails(t, "session", "resume", "plan")
	assertContains(t, h.err.String(), "cannot tell whether")
	h.err.Reset()
	h.fails(t, "session", "mv", "plan", to)
	assertContains(t, h.err.String(), "pass --force")
	h.ok(t, "session", "mv", "--force", "plan", to)
	if len(h.execs) != 0 {
		t.Fatalf("execs %q", h.execs)
	}
}

func TestSessionCommandsRefuseAnEmptyTarget(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "", h.folder("from"), time.Hour)
	h.fails(t, "session", "show", "")
	assertContains(t, h.err.String(), "empty target")
}

func TestSessionMvRmDeletesTheOriginalOnceCopied(t *testing.T) {
	h := newHarness(t)
	from, to := h.folder("from"), h.folder("to")
	id := h.transcript(t, "plan", from, time.Hour)
	src := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(from), id+".jsonl")
	writeFile(t, filepath.Join(strings.TrimSuffix(src, ".jsonl"), "subagents", "a.jsonl"), "{}\n")

	h.ok(t, "session", "mv", "--rm", "plan", to)
	for _, gone := range []string{src, strings.TrimSuffix(src, ".jsonl")} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s is still there: %v", gone, err)
		}
	}
	dest := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(to))
	for _, kept := range []string{filepath.Join(dest, id+".jsonl"), filepath.Join(dest, id, "subagents", "a.jsonl")} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatal(err)
		}
	}
	assertContains(t, h.out.String(), "moved the original from")
	if len(h.trashed) != 2 {
		t.Fatalf("trashed %q, want the transcript and its sidecar", h.trashed)
	}
}

func TestSessionMvRmPurgeDeletesForGood(t *testing.T) {
	h := newHarness(t)
	from := h.folder("from")
	id := h.transcript(t, "plan", from, time.Hour)
	src := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(from), id+".jsonl")

	h.fails(t, "session", "mv", "--purge", "plan", h.folder("to"))
	assertContains(t, h.err.String(), "--purge goes with --rm")
	h.ok(t, "session", "mv", "--rm", "--purge", "plan", h.folder("to"))
	if len(h.trashed) != 0 || exists(src) {
		t.Fatalf("trashed %q, or kept %s", h.trashed, src)
	}
	assertContains(t, h.out.String(), "deleted the original from")
}

func TestSessionMvRmRefusesTheHost(t *testing.T) {
	h := newHarness(t)
	host := h.bg("host", "busy")
	h.transcriptAs(t, host.SessionID, "host", h.folder("h"), time.Minute)
	h.agents.snapshots = [][]claude.Session{{host}}
	h.host(host)

	h.fails(t, "session", "mv", "--rm", "host", h.folder("to"))
	assertContains(t, h.err.String(), "Drop --rm")
}

func TestSessionResumeBgRefusesAJobThatWouldWakeInTheOldFolder(t *testing.T) {
	h := newHarness(t)
	from, to := h.folder("from"), h.folder("to")
	id := h.transcript(t, "plan", from, time.Hour)
	h.ok(t, "session", "mv", "plan", to)
	job := claude.ShortID(id)
	writeFile(t, filepath.Join(h.app.Jobs.Dir, job, "state.json"), fmt.Sprintf(`{"sessionId":%q,"cwd":%q}`, id, from))

	h.fails(t, "session", "resume", "--bg", "plan")
	assertContains(t, h.err.String(), "would wake it there")
	assertCalls(t, h)
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
	writeFile(t, filepath.Join(h.app.Jobs.Dir, s.JobID, "state.json"),
		fmt.Sprintf(`{"sessionId":%q,"cwd":%q,"respawnFlags":%s}`, s.SessionID, s.Cwd, flags))
	h.agents.finished = append(h.agents.finished, s)
	return s
}

func TestSessionMvRemovesAStoppedJobAndResumeBgStartsOneWithItsOptions(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	s := h.stoppedJob(t, "plan", "--permission-mode", "plan", "--model", "m")

	h.ok(t, "session", "mv", "plan", to)
	assertCalls(t, h, "rm "+s.JobID)
	assertContains(t, h.out.String(), "removed its stopped background job "+s.JobID)
	assertContains(t, h.out.String(), "with --permission-mode plan --model m")
	if _, err := os.Stat(filepath.Join(h.app.Jobs.Dir, s.JobID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the job is still there: %v", err)
	}

	h.agents.calls = nil
	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+s.SessionID+" in to prompt= name=plan options=--permission-mode plan --model m")

	// Once the new job exists, it wakes with its own options.
	h.agents.calls = nil
	h.agents.finished = []claude.Session{{JobID: claude.ShortID(s.SessionID), SessionID: s.SessionID, Cwd: to, Kind: "background", State: "done"}}
	h.agents.resumed = nil
	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+s.SessionID+" in to prompt=")
}

func TestSessionMvCarriesTheOptionsOnToTheNextMove(t *testing.T) {
	h := newHarness(t)
	s := h.stoppedJob(t, "plan", "--model", "m")
	// --rm leaves one copy, so the second move takes the first move's copy.
	h.ok(t, "session", "mv", "--rm", "plan", h.folder("to"))
	h.ok(t, "session", "mv", "--rm", "plan", h.folder("next"))

	h.agents.calls = nil
	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+s.SessionID+" in next prompt= name=plan options=--model m")
}

func TestSessionMvKeepsAJobItCannotRemove(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	s := h.stoppedJob(t, "plan", "--model", "m")
	h.agents.failures = map[string]string{"rm " + s.JobID: "busy"}

	h.fails(t, "session", "mv", "plan", to)
	assertContains(t, h.err.String(), "claudes gc "+s.JobID)
	if _, err := os.Stat(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(to), s.SessionID+".jsonl")); err != nil {
		t.Fatal(err)
	}

	// The options stay with the copy, for after the job is removed by hand.
	h.agents.failures = nil
	h.ok(t, "gc", s.JobID)
	h.agents.calls = nil
	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+s.SessionID+" in to prompt= name=plan options=--model m")
}

func TestSessionMvLeavesAJobThatIsNotStoppedOrIsPaused(t *testing.T) {
	for _, c := range []struct {
		name, want string
		setup      func(h *harness, s *claude.Session)
	}{
		{"working", "it is not stopped (working)", func(_ *harness, s *claude.Session) { s.State = "working" }},
		{"running", "it is not stopped", func(_ *harness, s *claude.Session) {
			// Started again after the transcripts were read.
			s.PID, s.Status, s.State = 4242, "idle", "done"
		}},
		{"paused", "it is paused", func(h *harness, s *claude.Session) {
			if err := h.app.Park.Save([]park.Entry{park.FromSession(*s, false, h.now.Format(time.RFC3339))}); err != nil {
				t.Fatal(err)
			}
		}},
		{"unreadable", "claudes cannot read the options", func(h *harness, s *claude.Session) {
			writeFile(t, filepath.Join(h.app.Jobs.Dir, s.JobID, "state.json"), "{")
		}},
		{"no respawnFlags", "claudes cannot read the options", func(h *harness, s *claude.Session) {
			writeFile(t, filepath.Join(h.app.Jobs.Dir, s.JobID, "state.json"), fmt.Sprintf(`{"sessionId":%q,"cwd":%q}`, s.SessionID, s.Cwd))
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			s := h.stoppedJob(t, "plan", "--model", "m")
			c.setup(h, &h.agents.finished[0])
			if c.name == "running" {
				// session mv lists running sessions first, and refuses one it sees.
				h.agents.snapshots = [][]claude.Session{{}, {h.agents.finished[0]}}
				h.agents.finished = nil
			}

			h.ok(t, "session", "mv", "plan", h.folder("to"))
			assertCalls(t, h)
			assertContains(t, h.out.String(), "kept its background job "+s.JobID+", since "+c.want)
			assertContains(t, h.out.String(), "wakes it in "+s.Cwd)
		})
	}
}

func TestSessionMvOfARunningBackgroundSessionRemovesNothing(t *testing.T) {
	h := newHarness(t)
	live := h.bg("live", "idle")
	h.transcriptAs(t, live.SessionID, "live", live.Cwd, time.Minute)
	writeFile(t, filepath.Join(h.app.Jobs.Dir, live.JobID, "state.json"), fmt.Sprintf(`{"sessionId":%q,"respawnFlags":["--model","m"]}`, live.SessionID))
	h.agents.snapshots = [][]claude.Session{{live}}

	h.fails(t, "session", "mv", "live", h.folder("to"))
	assertContains(t, h.err.String(), "Stop it first")
	assertCalls(t, h)
}

func TestSessionMvOfASessionWithNoJobRemovesNothing(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	id := h.transcript(t, "plan", h.folder("from"), time.Hour)
	h.agents.finished = []claude.Session{stopped(h.tty("other", "idle"))}

	h.ok(t, "session", "mv", "plan", to)
	assertCalls(t, h)
	if strings.Contains(h.out.String(), "background job") {
		t.Fatalf("output names a job:\n%s", h.out.String())
	}
	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+id+" in to prompt= name=plan")
}

func TestAnUnnamedLiveSessionKeepsItsTranscriptName(t *testing.T) {
	h := newHarness(t)
	live := h.bg("plan", "idle")
	h.transcriptAs(t, live.SessionID, "plan", h.folder("plan"), time.Minute)
	live.Name = live.JobID
	h.agents.snapshots = [][]claude.Session{{live}}

	h.ok(t, "session", "show", "plan")
	assertContains(t, h.out.String(), "plan")
}

func TestSessionResumeBgNamesASessionThatHasNoJob(t *testing.T) {
	h := newHarness(t)
	id := h.transcript(t, "plan", h.folder("work"), time.Hour)

	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+id+" in work prompt= name=plan")

	writeFile(t, filepath.Join(h.app.Jobs.Dir, claude.ShortID(id), "state.json"), fmt.Sprintf(`{"sessionId":%q}`, id))
	h.agents.calls = nil
	h.agents.resumed = nil
	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+id+" in work prompt=")
}

func TestSessionWhereTellsSessionsWithOneNameApart(t *testing.T) {
	h := newHarness(t)
	a := h.transcript(t, "plan", h.folder("one"), time.Hour)
	b := h.transcript(t, "plan", h.folder("two"), 2*time.Hour)

	h.ok(t, "session", "where", "plan")
	for _, id := range []string{a, b} {
		assertContains(t, h.out.String(), claude.ShortID(id))
	}
}

func TestSessionShowWritesTimesInUTCAndAnEmptyCopyList(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("one"), time.Hour)

	h.ok(t, "session", "show", "plan", "-o", "json")
	var got map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, h.out)
	}
	if copies, ok := got["copies"].([]any); !ok || len(copies) != 0 {
		t.Fatalf("copies %#v, want []", got["copies"])
	}
	if s, _ := got["lastActive"].(string); !strings.HasSuffix(s, "Z") {
		t.Fatalf("lastActive %q, want UTC", s)
	}
}

// A project directory's name reaches session show's older copies as is, so
// an escape sequence in it must not reach the terminal.
func TestSessionShowPrintsOlderCopiesPrintable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file names cannot hold control characters")
	}
	h := newHarness(t)
	id := h.transcript(t, "plan", h.folder("one"), time.Hour)
	src := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(h.folder("one")), id+".jsonl")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	evil := filepath.Join(h.app.Transcripts.Dir, "-evil\x1b[2J", id+".jsonl")
	writeFile(t, evil, string(data))
	old := h.now.Add(-48 * time.Hour)
	if err := os.Chtimes(evil, old, old); err != nil {
		t.Fatal(err)
	}

	h.ok(t, "session", "show", "plan")
	assertContains(t, h.out.String(), "older copy:")
	if strings.ContainsRune(h.out.String(), 0x1b) {
		t.Fatalf("session show printed an escape sequence: %q", h.out.String())
	}
}

// A transcript can record its directory through a link, as macOS does for the
// temporary directory under /var.
func TestSessionCommandsSeeAFolderRecordedThroughALink(t *testing.T) {
	h := newHarness(t)
	real := h.folder("real")
	link := filepath.Join(h.root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot make a link: %v", err)
	}
	h.transcript(t, "plan", link, time.Hour)

	h.ok(t, "session", "ls", "--in", real)
	assertContains(t, h.out.String(), "plan")

	h.fails(t, "session", "mv", "plan", real)
	assertContains(t, h.err.String(), "already lives under")
}

func TestSessionLsShowsSizesAndSortsBySize(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	h.transcript(t, "small", work, time.Hour)
	big := h.transcript(t, "big", work, 48*time.Hour)
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), big, "tool-results", "r.txt"), strings.Repeat("x", 4096))
	writeFile(t, filepath.Join(h.root, "file-history", big, "f@v1"), strings.Repeat("y", 1024))
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "ls")
	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if !strings.Contains(lines[0], "SIZE") || !strings.HasSuffix(lines[1], "small") {
		t.Fatalf("table:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "--sort", "size")
	lines = strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if !strings.HasSuffix(lines[1], "big") || !strings.Contains(lines[1], "5.") {
		t.Fatalf("--sort size:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "big", "-o", "json")
	var rows []pastSession
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if d := rows[0].Disk; len(rows) != 1 || d.Sidecar != 4096 || d.FileHistory != 1024 || d.Total != d.Transcript+5120 {
		t.Fatalf("rows %+v", rows)
	}

	h.fails(t, "session", "ls", "--sort", "name")
	if !strings.Contains(h.err.String(), `for "--sort" flag: must be active or size, not "name"`) {
		t.Fatalf("stderr:\n%s", h.err)
	}
}

func TestSessionDuAddsUpByFolderWithLeftovers(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	a := h.transcript(t, "a", work, time.Hour)
	h.transcript(t, "b", work, 2*time.Hour)
	h.transcript(t, "c", other, 3*time.Hour)
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), a, "tool-results", "r.txt"), strings.Repeat("x", 2048))
	orphan := sessionID("orphan")
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), orphan, "tool-results", "r.txt"), strings.Repeat("z", 300))
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), "memory", "MEMORY.md"), "notes")
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "du", "-o", "json")
	var report duReport
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Dirs) != 2 || report.Dirs[0].Dir != work || report.Dirs[0].Sessions != 2 ||
		report.Dirs[0].Disk.Sidecar != 2048 || report.Dirs[1].Sessions != 1 {
		t.Fatalf("directories %+v", report.Dirs)
	}
	if len(report.Leftovers) != 1 || report.Leftovers[0].SessionID != orphan || report.Leftovers[0].Size != 300 {
		t.Fatalf("leftovers %+v", report.Leftovers)
	}
	if want := report.Dirs[0].Disk.Total + report.Dirs[1].Disk.Total + 300; report.Total != want {
		t.Fatalf("total %d, want %d", report.Total, want)
	}

	h.out.Reset()
	h.ok(t, "session", "df")
	text := h.out.String()
	if !strings.Contains(text, "1 leftover dir(s)") || !strings.Contains(text, "all") {
		t.Fatalf("df:\n%s", text)
	}

	h.out.Reset()
	h.ok(t, "session", "du", "--in", other, "-o", "json")
	report = duReport{}
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Dirs) != 1 || len(report.Leftovers) != 0 {
		t.Fatalf("--in counted other directories or leftovers: %+v", report)
	}
}

func TestSessionShowPrintsTheSize(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("work"), time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "show", "plan")
	if !strings.Contains(h.out.String(), "size:") || !strings.Contains(h.out.String(), "file history 0B") {
		t.Fatalf("show:\n%s", h.out)
	}
}

func TestSessionDuTopListsTheBiggestSessionsWithTheirSidecarShare(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	h.transcript(t, "small", work, time.Hour)
	big := h.transcript(t, "big", work, 2*time.Hour)
	mid := h.transcript(t, "mid", work, 3*time.Hour)
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), big, "tool-results", "r.txt"), strings.Repeat("x", 9000))
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), mid, "tool-results", "r.txt"), strings.Repeat("x", 3000))
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "du", "--top", "2", "-o", "json")
	var report duReport
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Top) != 2 || report.Top[0].SessionID != big || report.Top[1].SessionID != mid || report.Top[0].Disk.Sidecar != 9000 {
		t.Fatalf("top %+v", report.Top)
	}

	h.out.Reset()
	h.ok(t, "session", "du", "--top", "5")
	text := h.out.String()
	if !strings.Contains(text, "The 3 biggest session(s):") || strings.Index(text, "big") > strings.Index(text, "small") {
		t.Fatalf("output:\n%s", text)
	}
	if !regexp.MustCompile(`(?m)8\.8K\s+9\d%\s+\S+\s+big$`).MatchString(text) {
		t.Fatalf("no sidecar share for big:\n%s", text)
	}

	h.out.Reset()
	h.ok(t, "session", "du", "-o", "json")
	if !strings.Contains(h.out.String(), `"dirs": [`) || !strings.Contains(h.out.String(), `"dir": "`) {
		t.Fatalf("du JSON does not use dirs and dir:\n%s", h.out)
	}
	if !strings.Contains(h.out.String(), `"top": []`) {
		t.Fatalf("top is not an empty list without --top:\n%s", h.out)
	}

	h.fails(t, "session", "du", "--top", "-1")
	assertContains(t, h.err.String(), `invalid argument "-1" for "-t, --top" flag: must be a whole number, 0 or more`)
}

func TestTablesShareOneColumnOrderAndCutLongNames(t *testing.T) {
	h := newHarness(t)
	long := strings.Repeat("A long title Claude Code wrote ", 3)
	live := h.bg("live", "idle")
	live.Name = long
	h.transcriptAs(t, live.SessionID, long, live.Cwd, 2*time.Hour)
	h.agents.snapshots = [][]claude.Session{{live}}

	header := regexp.MustCompile(`^ID\s.*\sDIR\s+NAME(\s+(NOTE|DOING))?$`)
	name := regexp.MustCompile(regexp.QuoteMeta(strings.TrimSpace(long[:maxName-1])+"…") + `(\s\s+\S+)?$`)
	for _, args := range [][]string{{"ls"}, {"ls", "--wide"}, {"session", "ls"}, {"session", "du", "--top", "1"}, {"restart", "--dry-run", live.JobID}} {
		h.out.Reset()
		h.ok(t, args...)
		var headed, short bool
		for line := range strings.Lines(h.out.String()) {
			line = strings.TrimSpace(line)
			headed = headed || header.MatchString(line)
			if strings.Contains(line, long[:20]) {
				short = name.MatchString(line)
			}
		}
		if !headed || !short {
			t.Errorf("%v: want ID first, then DIR, a name cut to %d characters, and at most a note:\n%s", args, maxName, h.out)
		}
	}
}
