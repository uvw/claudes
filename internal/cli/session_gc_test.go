package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/transcripts"
)

// emptyTranscript writes a transcript for a session that never got a prompt,
// last active ago before now, and returns its session id.
func (h *harness) emptyTranscript(t *testing.T, name, cwd string, ago time.Duration) string {
	t.Helper()
	id := sessionID("empty " + name + cwd + ago.String())
	var b strings.Builder
	for _, r := range []map[string]any{
		{"type": "system", "sessionId": id, "cwd": cwd, "timestamp": h.now.Add(-ago).Format(time.RFC3339)},
		{"type": "last-prompt", "lastPrompt": "from an earlier session", "sessionId": id},
	} {
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

// aged sets every file and directory under path to have last changed ago before
// now.
func (h *harness) aged(t *testing.T, path string, ago time.Duration) {
	t.Helper()
	at := h.now.Add(-ago)
	err := filepath.Walk(path, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, at, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestSessionGcRemovesEmptySessionsCopiesAndLeftovers(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	week := 7 * 24 * time.Hour

	empty := h.emptyTranscript(t, "e", work, week)
	emptyPath := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), empty+".jsonl")
	history := filepath.Join(h.root, "file-history", empty, "f@v1")
	writeFile(t, history, "v1")
	h.aged(t, filepath.Dir(history), week)
	fresh := h.emptyTranscript(t, "fresh", work, time.Hour)
	kept := h.transcript(t, "kept", work, week)

	moved := h.transcript(t, "moved", other, 48*time.Hour)
	oldCopy := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), moved+".jsonl")
	data, err := os.ReadFile(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), moved+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, oldCopy, string(data))
	writeFile(t, filepath.Join(strings.TrimSuffix(oldCopy, ".jsonl"), "tool-results", "r.txt"), "result")
	h.aged(t, strings.TrimSuffix(oldCopy, ".jsonl"), week)
	h.aged(t, oldCopy, week)
	moves := map[string]string{strings.ToLower(transcripts.Folder(work)) + "/" + moved: work, "keep/" + moved: other}
	movesData, _ := json.Marshal(moves)
	writeFile(t, h.app.Transcripts.Moves, string(movesData))

	orphan := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), sessionID("orphan"))
	writeFile(t, filepath.Join(orphan, "subagents", "a.jsonl"), "{}")
	h.aged(t, orphan, week)
	freshOrphan := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), sessionID("fresh orphan"))
	writeFile(t, filepath.Join(freshOrphan, "subagents", "a.jsonl"), "{}")
	memory := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), "memory", "MEMORY.md")
	writeFile(t, memory, "notes")
	h.aged(t, filepath.Dir(memory), week)
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "gc")

	for _, gone := range []string{emptyPath, filepath.Dir(history), oldCopy, strings.TrimSuffix(oldCopy, ".jsonl"), orphan} {
		if exists(gone) {
			t.Errorf("%s is still there", gone)
		}
		if !slices.Contains(h.trashed, gone) {
			t.Errorf("%s did not go to the trash: %q", gone, h.trashed)
		}
	}
	for _, still := range []string{
		filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), fresh+".jsonl"),
		filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), kept+".jsonl"),
		filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), moved+".jsonl"),
		freshOrphan, memory,
	} {
		if !exists(still) {
			t.Errorf("%s was removed", still)
		}
	}
	left, err := os.ReadFile(h.app.Transcripts.Moves)
	if err != nil || strings.Contains(string(left), strings.ToLower(transcripts.Folder(work))) || !strings.Contains(string(left), "keep/") {
		t.Fatalf("moves.json %s, err %v", left, err)
	}
	if !strings.Contains(h.out.String(), "Freed") {
		t.Fatalf("output:\n%s", h.out)
	}
}

func TestSessionGcLeavesProtectedSessionsAlone(t *testing.T) {
	h := newHarness(t)
	week := 7 * 24 * time.Hour
	live := h.bg("live", "idle")
	h.emptyTranscriptAs(t, live.SessionID, live.Cwd, week)
	listed := stopped(h.bg("listed", ""))
	h.emptyTranscriptAs(t, listed.SessionID, listed.Cwd, week)
	parked := h.emptyTranscript(t, "parked", h.folder("p"), week)
	host := h.emptyTranscript(t, "host", h.folder("h"), week)
	h.app.HostID = host
	if err := h.app.Park.Save([]park.Entry{{SessionID: parked, Name: "parked", Cwd: h.folder("p"), Kind: "background"}}); err != nil {
		t.Fatal(err)
	}
	h.agents.snapshots = [][]claude.Session{{live}}
	h.agents.finished = []claude.Session{listed}

	h.ok(t, "session", "gc")
	if len(h.trashed) != 0 {
		t.Fatalf("trashed %q", h.trashed)
	}
	if !strings.Contains(h.out.String(), "Nothing to clean up.") {
		t.Fatalf("output:\n%s", h.out)
	}

	for _, name := range []string{live.SessionID[:8], listed.SessionID[:8], parked[:8], host[:8]} {
		h.err.Reset()
		if code := h.run("session", "gc", name); code != exitRefused {
			t.Fatalf("gc %s: exit %d\n%s", name, code, h.err)
		}
		if !strings.Contains(h.err.String(), "session gc leaves it alone") {
			t.Fatalf("gc %s:\n%s", name, h.err)
		}
	}
}

// emptyTranscriptAs writes an empty transcript for a given session id.
func (h *harness) emptyTranscriptAs(t *testing.T, id, cwd string, ago time.Duration) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"type": "system", "sessionId": id, "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(cwd), id+".jsonl")
	writeFile(t, path, string(data)+"\n")
	h.aged(t, path, ago)
}

func TestSessionGcKindsTargetsPurgeAndDryRun(t *testing.T) {
	h := newHarness(t)
	week := 7 * 24 * time.Hour
	gone := filepath.Join(h.root, "deleted-directory")
	goneID := h.transcript(t, "gone", gone, week)
	old := h.transcript(t, "old", h.folder("work"), 100*24*time.Hour)
	recentOld := h.transcript(t, "recent", h.folder("work"), 2*time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "gc", "--gone", "--dry-run")
	if !strings.Contains(h.out.String(), "gone") || strings.Contains(h.out.String(), "old ") || len(h.trashed) != 0 {
		t.Fatalf("--gone --dry-run:\n%s", h.out)
	}
	if !exists(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(gone), goneID+".jsonl")) {
		t.Fatal("a dry run removed something")
	}

	h.out.Reset()
	h.ok(t, "session", "gc", "--older-than", "90d", "--purge")
	if len(h.trashed) != 0 {
		t.Fatalf("--purge used the trash: %q", h.trashed)
	}
	if exists(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(h.folder("work")), old+".jsonl")) {
		t.Fatal("--older-than left the old session")
	}
	if !exists(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(gone), goneID+".jsonl")) {
		t.Fatal("--older-than removed a session of another kind")
	}

	h.out.Reset()
	h.ok(t, "session", "gc", recentOld[:8])
	if exists(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(h.folder("work")), recentOld+".jsonl")) {
		t.Fatal("a named session active in the last day was kept")
	}

	h.fails(t, "session", "gc", "--empty", goneID[:8])
	if !strings.Contains(h.err.String(), "targets or kinds") {
		t.Fatalf("stderr:\n%s", h.err)
	}
}

func TestSessionGcSkipsASessionThatStartedRunningSinceThePlan(t *testing.T) {
	h := newHarness(t)
	s := stopped(h.bg("waking", ""))
	h.emptyTranscriptAs(t, s.SessionID, s.Cwd, 7*24*time.Hour)
	running := s
	running.PID, running.Status, running.State = 4242, "idle", "done"
	// Listed for pastSessions and for gcKeep, then running by the time
	// gc removes.
	h.agents.snapshots = [][]claude.Session{{}, {}, {running}}

	h.ok(t, "session", "gc")
	if len(h.trashed) != 0 || !strings.Contains(h.out.String(), "it started running") {
		t.Fatalf("trashed %q:\n%s", h.trashed, h.out)
	}
}

func TestSessionGcRefusesAPathOutsideClaudesFolder(t *testing.T) {
	h := newHarness(t)
	outside := filepath.Join(t.TempDir(), sessionID("x"))
	writeFile(t, filepath.Join(outside, "f"), "keep me")
	it := gcItem{sessionID: sessionID("x"), name: "x", paths: []string{outside}}
	if err := h.app.Transcripts.Remove(it.sessionID, it.paths, os.RemoveAll); err == nil || !exists(outside) {
		t.Fatalf("removed %s: %v", outside, err)
	}
	inside := filepath.Join(h.root, "projects", "-w", "not-the-session")
	writeFile(t, filepath.Join(inside, "f"), "keep me")
	it.paths = []string{inside}
	if err := h.app.Transcripts.Remove(it.sessionID, it.paths, os.RemoveAll); err == nil || !exists(inside) {
		t.Fatalf("removed %s: %v", inside, err)
	}
}

func TestSessionGcRefusesWhenItCannotTellWhatRuns(t *testing.T) {
	h := newHarness(t)
	h.emptyTranscript(t, "e", h.folder("work"), 7*24*time.Hour)
	h.agents.failures = map[string]string{"list": "claude agents failed"}

	if code := h.run("session", "gc"); code != exitRefused {
		t.Fatalf("exit %d\n%s", code, h.err)
	}
	if len(h.trashed) != 0 {
		t.Fatalf("trashed %q", h.trashed)
	}
}

// sessionOf writes a transcript of records for a new session in cwd, last
// active ago before now, and returns its session id.
func (h *harness) sessionOf(t *testing.T, name, cwd string, ago time.Duration, records ...string) string {
	t.Helper()
	id := sessionID("records " + name)
	lines := []string{fmt.Sprintf(`{"type":"system","sessionId":%q,"cwd":%q}`, id, cwd)}
	lines = append(lines, records...)
	path := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(cwd), id+".jsonl")
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	h.aged(t, path, ago)
	return id
}

const (
	slashLogin = `{"type":"user","message":{"content":"<command-name>/login</command-name>"}}`
	toolCall   = `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{}}]}}`
	editCall   = `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{}}]}}`
	reply      = `{"type":"assistant","message":{"content":[{"type":"text","text":"Pong."}]}}`
)

func prompt(text string) string {
	return fmt.Sprintf(`{"type":"user","message":{"content":%q}}`, text)
}

func TestSessionGcTrivialTakesSessionsThatDidNothing(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	week := 7 * 24 * time.Hour
	picked := map[string]string{
		"slash only": h.sessionOf(t, "slash only", work, week, slashLogin),
		"ping":       h.sessionOf(t, "ping", work, week, prompt("ping"), reply),
	}
	kept := map[string]string{
		"one prompt, a tool call": h.sessionOf(t, "tool", work, week, prompt("what branch is this?"), toolCall),
		"slash then a tool call":  h.sessionOf(t, "skill", work, week, slashLogin, toolCall),
		"two prompts":             h.sessionOf(t, "two", work, week, prompt("hi"), reply, prompt("and?"), reply),
		"an edit":                 h.sessionOf(t, "edit", work, week, prompt("fix it"), editCall),
		"renamed":                 h.sessionOf(t, "renamed", work, week, prompt("ping"), `{"type":"custom-title","customTitle":"keep me"}`),
		"teammate":                h.sessionOf(t, "teammate", work, week, `{"type":"user","message":{"content":"<teammate-message teammate_id=\"lead\">go</teammate-message>"}}`),
		"teammate with a prompt": h.sessionOf(t, "teammate prompt", work, week,
			`{"type":"user","message":{"content":"<teammate-message teammate_id=\"lead\">go</teammate-message>"}}`, prompt("ping")),
		"no prompt it could read": h.sessionOf(t, "unread", work, week, reply),
		"active today":            h.sessionOf(t, "today", work, time.Hour, prompt("ping")),
		"a record type claudes does not know": h.sessionOf(t, "drift", work, week, prompt("ping"),
			`{"type":"assistant-turn","message":{"content":[{"type":"tool_use","name":"Edit","input":{}}]}}`),
		"a line that does not parse": h.sessionOf(t, "cut", work, week, prompt("ping"), `{"type":"assistant","mess`),
	}
	history := h.sessionOf(t, "history", work, week, prompt("ping"))
	kept["file history"] = history
	writeFile(t, filepath.Join(h.root, "file-history", history, "f@v1"), "v1")
	big := h.sessionOf(t, "big", work, week, prompt(strings.Repeat("x", transcripts.ActivityLimit)))
	kept["too big to count"] = big
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "gc")
	if len(h.trashed) != 0 {
		t.Fatalf("gc without --trivial trashed %q", h.trashed)
	}

	h.ok(t, "session", "gc", "--trivial")
	for what, id := range picked {
		if !slices.ContainsFunc(h.trashed, func(p string) bool { return strings.Contains(p, id) }) {
			t.Errorf("%s was not trashed", what)
		}
	}
	for what, id := range kept {
		if slices.ContainsFunc(h.trashed, func(p string) bool { return strings.Contains(p, id) }) {
			t.Errorf("%s was trashed", what)
		}
	}
	if !strings.Contains(h.out.String(), "trivial") {
		t.Errorf("the plan does not say why:\n%s", h.out)
	}
}

func TestSessionShowSaysWhatASessionDid(t *testing.T) {
	h := newHarness(t)
	h.sessionOf(t, "did", h.folder("work"), time.Hour, prompt("fix the\nflaky test"), editCall, `{"type":"custom-title","customTitle":"flaky"}`)
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "show", "flaky")
	for _, want := range []string{"flaky (set with /rename)", "1 prompt, 0 commands, 1 tool call, 1 edit", "fix the flaky test"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("no %q in:\n%s", want, h.out)
		}
	}

	h.out.Reset()
	h.ok(t, "session", "show", "flaky", "-o", "json")
	var got struct {
		NameSource string               `json:"nameSource"`
		Activity   transcripts.Activity `json:"activity"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.NameSource != "custom" || got.Activity.Prompts != 1 || got.Activity.Edits != 1 {
		t.Errorf("json: %+v", got)
	}
}

func TestSessionGcSidecarsKeepsTheSessionAndTrimsItsSidecar(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	sidecarOf := func(id string) string {
		return filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), id)
	}
	old := h.transcript(t, "old", work, 60*24*time.Hour)
	recent := h.transcript(t, "recent", work, 3*24*time.Hour)
	today := h.transcript(t, "today", work, time.Hour)
	for _, id := range []string{old, recent, today} {
		writeFile(t, filepath.Join(sidecarOf(id), "tool-results", "r.txt"), strings.Repeat("x", 500))
		writeFile(t, filepath.Join(sidecarOf(id), "subagents", "agent-a1.jsonl"), "{}")
		h.aged(t, sidecarOf(id), time.Hour)
	}
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "gc", "--sidecars")
	for _, gone := range []string{filepath.Join(sidecarOf(old), "tool-results"), filepath.Join(sidecarOf(recent), "tool-results")} {
		if exists(gone) || !slices.Contains(h.trashed, gone) {
			t.Errorf("%s was not trashed: %q", gone, h.trashed)
		}
	}
	for _, still := range []string{
		filepath.Join(sidecarOf(old), "subagents"), filepath.Join(sidecarOf(today), "tool-results"),
		sidecarOf(old) + ".jsonl", sidecarOf(recent) + ".jsonl",
	} {
		if !exists(still) {
			t.Errorf("%s was removed", still)
		}
	}

	h.trashed = nil
	h.ok(t, "session", "gc", "--sidecars", "--older-than", "30d")
	if want := []string{filepath.Join(sidecarOf(old), "subagents")}; !slices.Equal(h.trashed, want) {
		t.Errorf("trashed %q, want %q", h.trashed, want)
	}
	if !exists(sidecarOf(old)+".jsonl") || !exists(filepath.Join(sidecarOf(recent), "subagents")) {
		t.Error("--sidecars --older-than removed a whole session or a recent session's subagents")
	}
}

func TestSessionGcRemovesOnlyFoldersInsideTheSessionsSidecar(t *testing.T) {
	h := newHarness(t)
	id := sessionID("x")
	mine := filepath.Join(h.root, "projects", "-w", id, "tool-results")
	writeFile(t, filepath.Join(mine, "r.txt"), "x")
	theirs := filepath.Join(h.root, "projects", "-w", sessionID("y"), "tool-results")
	writeFile(t, filepath.Join(theirs, "r.txt"), "keep me")

	if err := h.app.Transcripts.Remove(id, []string{theirs}, os.RemoveAll); err == nil || !exists(theirs) {
		t.Fatalf("removed another session's directory: %v", err)
	}
	if err := h.app.Transcripts.Remove(id, []string{mine}, os.RemoveAll); err != nil || exists(mine) {
		t.Fatalf("kept the session's own directory: %v", err)
	}
}

func TestSessionGcSkipsASessionThatChangedSinceThePlan(t *testing.T) {
	h := newHarness(t)
	id := h.emptyTranscript(t, "e", h.folder("work"), 7*24*time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}
	list, err := h.app.pastSessions(t.Context())
	if err != nil || len(list) != 1 {
		t.Fatalf("list %+v, err %v", list, err)
	}
	it := h.app.wholeSession(list[0], "empty")
	if err := os.Chtimes(list[0].Path, h.now, h.now); err != nil {
		t.Fatal(err)
	}

	if err := h.app.gcRemove(t.Context(), []gcItem{it}, false); err != nil {
		t.Fatal(err)
	}
	if len(h.trashed) != 0 || !exists(list[0].Path) {
		t.Fatalf("trashed %q", h.trashed)
	}
	assertContains(t, h.out.String(), "it changed since the plan")
	assertContains(t, h.out.String(), id[:8])
}

// An older copy that changed in the last day, such as one resumed from its old
// directory after claudes listed the sessions, is left alone.
func TestSessionGcKeepsAnOlderCopyThatChangedInTheLastDay(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	week := 7 * 24 * time.Hour
	moved := h.transcript(t, "moved", other, 2*week)
	data, err := os.ReadFile(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), moved+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	oldCopy := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), moved+".jsonl")
	writeFile(t, oldCopy, string(data))
	h.aged(t, oldCopy, 3*week)
	h.agents.snapshots = [][]claude.Session{{}}
	list, err := h.app.pastSessions(t.Context())
	if err != nil || len(list) != 1 || len(list[0].Copies) != 1 {
		t.Fatalf("list %+v, err %v", list, err)
	}
	h.aged(t, oldCopy, 2*time.Hour)

	items, err := h.app.gcPick(list, map[string]string{}, gcFlags{copies: true})
	if err != nil || len(items) != 0 {
		t.Fatalf("picked %+v, err %v", items, err)
	}
}

// A directory claudes cannot check, such as one behind a permission error, may
// still be there, so --gone does not take its sessions.
func TestSessionGcGoneSkipsADirectoryItCannotCheck(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the test cannot search")
	}
	h := newHarness(t)
	locked := filepath.Join(h.root, "locked")
	cwd := filepath.Join(locked, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	h.transcript(t, "behind", cwd, 7*24*time.Hour)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "gc", "--gone")
	if len(h.trashed) != 0 {
		t.Fatalf("trashed %q", h.trashed)
	}
	assertContains(t, h.out.String(), "Nothing to clean up.")
}

func TestSessionGcConfirmAsksFirst(t *testing.T) {
	h := newHarness(t)
	id := h.emptyTranscript(t, "empty", h.folder("work"), 7*24*time.Hour)
	path := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(h.folder("work")), id+".jsonl")
	h.agents.snapshots = [][]claude.Session{{}}

	h.app.In = strings.NewReader("n\n")
	h.fails(t, "session", "gc", "--confirm")
	if !exists(path) || len(h.trashed) != 0 {
		t.Fatalf("session gc --confirm removed what it was told not to: %q", h.trashed)
	}
	assertContains(t, h.out.String(), "Move these to the trash? [y/N]")

	h.app.In = strings.NewReader("y\n")
	h.ok(t, "session", "gc", "--confirm", "--purge")
	assertContains(t, h.out.String(), "Delete these for good? [y/N]")
	if exists(path) {
		t.Fatal("session gc --confirm kept a session after a yes")
	}
}
