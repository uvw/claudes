package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/files"
	"github.com/uvw/claudes/internal/teams"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/trash"
)

// trashedSession writes a session named plan with files in each place Claude
// Code keeps them, removes it with session gc, and returns its id and those
// files.
func (h *harness) trashedSession(t *testing.T) (string, []string) {
	t.Helper()
	work := h.folder("plan")
	id := h.transcript(t, "plan", work, 7*24*time.Hour)
	transcript := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), id+".jsonl")
	files := []string{
		transcript,
		filepath.Join(strings.TrimSuffix(transcript, ".jsonl"), "subagents", "a.jsonl"),
		filepath.Join(h.root, "file-history", id, "f@v1"),
		filepath.Join(h.root, "session-env", id, "env"),
		filepath.Join(h.root, "debug", id+".txt"),
	}
	for _, f := range files[1:] {
		writeFile(t, f, "kept")
	}
	h.agents.snapshots = [][]claude.Session{{}}
	h.ok(t, "session", "gc", id[:8])
	for _, f := range files {
		if exists(f) {
			t.Fatalf("session gc left %s", f)
		}
	}
	h.out.Reset()
	return id, files
}

func TestSessionRestorePutsBackWhatSessionGcTrashed(t *testing.T) {
	h := newHarness(t)
	id, files := h.trashedSession(t)

	h.ok(t, "session", "restore")
	assertContains(t, h.out.String(), id[:8])
	assertContains(t, h.out.String(), "TRASHED")
	assertContains(t, h.out.String(), "plan")

	h.out.Reset()
	h.ok(t, "session", "restore", "-o", "json")
	var listed []trash.Entry
	if err := json.Unmarshal(h.out.Bytes(), &listed); err != nil || len(listed) != 1 || listed[0].SessionID != id || listed[0].ShortID != id[:8] ||
		len(listed[0].Files) != 5 || listed[0].Files[0].Command != "session gc" || listed[0].Size == 0 {
		t.Fatalf("json %+v, err %v:\n%s", listed, err, h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "restore", "--dry-run", "plan")
	assertContains(t, h.out.String(), "Puts back 5 file(s)")
	assertContains(t, h.out.String(), files[0])
	assertContains(t, h.out.String(), "Dry run: nothing was changed.")
	if exists(files[0]) {
		t.Fatal("a dry run put a file back")
	}

	h.out.Reset()
	h.ok(t, "session", "restore", id[:8])
	assertContains(t, h.out.String(), "restored")
	for _, f := range files {
		if data, err := os.ReadFile(f); err != nil || len(data) == 0 {
			t.Errorf("%s did not come back: %v", f, err)
		}
	}
	if exists(h.app.Trashed.Path) {
		t.Fatal("trashed.json still lists the restored session")
	}
	h.out.Reset()
	h.ok(t, "session", "where", "--one", "plan")
	if got := strings.TrimSpace(h.out.String()); got != h.folder("plan") {
		t.Fatalf("where %q after the restore", got)
	}
	h.out.Reset()
	h.ok(t, "session", "restore")
	assertContains(t, h.out.String(), "Nothing to restore.")
}

func TestSessionRestoreRefusesATakenPathAndAMissingFile(t *testing.T) {
	h := newHarness(t)
	id, files := h.trashedSession(t)
	writeFile(t, files[4], "a new debug log")

	h.exits(t, exitRefused, "session", "restore", id[:8])
	assertContains(t, h.err.String(), "session restore put nothing back")
	assertContains(t, h.err.String(), files[4]+" exists")
	if exists(files[0]) {
		t.Fatal("a refused restore put the transcript back")
	}
	if data, _ := os.ReadFile(files[4]); string(data) != "a new debug log" {
		t.Fatalf("the debug log there was replaced with %q", data)
	}

	if err := os.Remove(files[4]); err != nil {
		t.Fatal(err)
	}
	entries, err := h.app.Trashed.Entries()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
	if err := os.RemoveAll(entries[0].Files[0].To); err != nil {
		t.Fatal(err)
	}
	h.err.Reset()
	h.exits(t, exitRefused, "session", "restore", id[:8])
	assertContains(t, h.err.String(), files[0]+" is no longer in the trash")

	h.out.Reset()
	h.ok(t, "session", "restore")
	assertContains(t, h.out.String(), "1 file(s) no longer in the trash")
}

func TestSessionGcPurgeRecordsNothing(t *testing.T) {
	h := newHarness(t)
	id := h.transcript(t, "plan", h.folder("plan"), 7*24*time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}
	h.ok(t, "session", "gc", "--purge", id[:8])
	if exists(h.app.Trashed.Path) {
		t.Fatal("--purge recorded what it deleted")
	}
	h.out.Reset()
	h.ok(t, "session", "restore")
	assertContains(t, h.out.String(), "Nothing to restore.")
	h.exits(t, exitNoMatch, "session", "restore", id[:8])
}

func TestSessionRestoreBringsBackAMovedSessionsMoves(t *testing.T) {
	h := newHarness(t)
	id := h.transcript(t, "plan", h.folder("from"), 7*24*time.Hour)
	h.ok(t, "session", "mv", "plan", h.folder("to"))
	h.agents.snapshots = [][]claude.Session{{}}
	h.ok(t, "session", "gc", id[:8])
	if data, _ := os.ReadFile(h.app.Transcripts.Moves); strings.Contains(string(data), id) {
		t.Fatalf("moves.json kept the removed session:\n%s", data)
	}

	h.ok(t, "session", "restore", id[:8])
	h.out.Reset()
	h.ok(t, "session", "where", "--one", "plan")
	if got := strings.TrimSpace(h.out.String()); got != h.folder("to") {
		t.Fatalf("where %q after the restore, want the directory it moved to", got)
	}
}

func TestSessionRestorePutsBackWhatSessionMvRmAndGcTeamsTrashed(t *testing.T) {
	h := newHarness(t)
	from := h.folder("from")
	id := h.transcript(t, "plan", from, 7*24*time.Hour)
	original := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(from), id+".jsonl")
	h.ok(t, "session", "mv", "--rm", "plan", h.folder("to"))
	tasks := filepath.Join(h.app.Teams.Tasks, teams.Name(id))
	writeFile(t, filepath.Join(tasks, "1.json"), "{}")
	h.aged(t, tasks, 30*24*time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}
	h.ok(t, "gc", "--teams")
	if exists(original) || exists(tasks) {
		t.Fatal("session mv --rm or gc --teams left its files")
	}

	h.out.Reset()
	h.ok(t, "session", "restore", "-o", "json")
	var listed []trash.Entry
	if err := json.Unmarshal(h.out.Bytes(), &listed); err != nil || len(listed) != 1 || len(listed[0].Files) != 2 {
		t.Fatalf("json %+v, err %v:\n%s", listed, err, h.out)
	}
	h.ok(t, "session", "restore", id[:8])
	if !exists(original) || !exists(filepath.Join(tasks, "1.json")) {
		t.Fatal("session restore did not put back the original and the task list")
	}
}

func TestSessionRestoreReportsAPartialRestore(t *testing.T) {
	h := newHarness(t)
	id, files := h.trashedSession(t)
	// A file where session-env should be keeps its directory from coming back.
	if err := os.RemoveAll(filepath.Join(h.root, "session-env")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.root, "session-env"), "in the way")

	h.exits(t, exitSomeFailed, "session", "restore", id[:8])
	assertContains(t, h.err.String(), "put back 4 of 5 file(s)")
	assertContains(t, h.err.String(), filepath.Dir(files[3])+" stayed in the trash")
	entries, err := h.app.Trashed.Entries()
	if err != nil || len(entries) != 1 || len(entries[0].Files) != 1 || entries[0].Files[0].From != filepath.Dir(files[3]) {
		t.Fatalf("entries after a partial restore %+v, err %v", entries, err)
	}

	if err := os.Remove(filepath.Join(h.root, "session-env")); err != nil {
		t.Fatal(err)
	}
	h.ok(t, "session", "restore", id[:8])
	if !exists(files[3]) || exists(h.app.Trashed.Path) {
		t.Fatal("the retry did not finish the restore")
	}
}

func TestSessionRestoreListsWhileAnotherClaudesRuns(t *testing.T) {
	h := newHarness(t)
	id, _ := h.trashedSession(t)
	release, err := files.Lock{Path: h.app.LockPath, Stale: time.Minute}.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	h.ok(t, "session", "restore")
	assertContains(t, h.out.String(), id[:8])
	h.exits(t, exitBusy, "session", "restore", id[:8])
}
