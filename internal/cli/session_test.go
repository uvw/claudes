package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
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
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "live") || !strings.Contains(lines[1], "running") ||
		!strings.HasPrefix(lines[2], "old-plan") || !strings.Contains(lines[2], "3d") {
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

	h.fails(t, "session", "where", "--one", "alpha")
	assertContains(t, h.err.String(), "matches 2 sessions")
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

	// The copy's records still name the old folder; claudes' own record of
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
	for shell, want := range map[string]string{"zsh": "command claudes session where --one", "pwsh": "Set-Location -LiteralPath $dir"} {
		h.out.Reset()
		h.ok(t, "init", shell)
		assertContains(t, h.out.String(), want)
	}
	h.fails(t, "init", "fish")
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
	assertContains(t, h.out.String(), "removed the original")
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

// A transcript can record its folder through a link, as macOS does for the
// temporary folder under /var.
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
