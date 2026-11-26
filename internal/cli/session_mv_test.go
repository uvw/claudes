package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/transcripts"
)

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

func TestSessionMoveIsMv(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("from"), time.Hour)
	h.ok(t, "session", "move", "plan", h.folder("to"))
	assertContains(t, h.out.String(), "copied plan")
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

func TestSessionMvRemovesAStoppedJobAndResumeBgStartsOneWithItsOptions(t *testing.T) {
	h := newHarness(t)
	to := h.folder("to")
	s := h.stoppedJob(t, "plan", "--permission-mode", "plan", "--model", "m")

	h.ok(t, "session", "mv", "plan", to)
	assertCalls(t, h, "rm "+s.JobID)
	assertContains(t, h.out.String(), "removed its stopped background job "+s.JobID)
	assertContains(t, h.out.String(), "with --permission-mode plan --model m")
	if _, err := os.Stat(filepath.Join(h.app.Accounts[0].Jobs.Dir, s.JobID)); !errors.Is(err, os.ErrNotExist) {
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
			writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, s.JobID, "state.json"), "{")
		}},
		{"no respawnFlags", "claudes cannot read the options", func(h *harness, s *claude.Session) {
			writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, s.JobID, "state.json"), fmt.Sprintf(`{"sessionId":%q,"cwd":%q}`, s.SessionID, s.Cwd))
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
	writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, live.JobID, "state.json"), fmt.Sprintf(`{"sessionId":%q,"respawnFlags":["--model","m"]}`, live.SessionID))
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
