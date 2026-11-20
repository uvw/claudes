package trash

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const sid = "abcd1234-1111-2222-3333-444455556666"

// world is a Claude Code directory and a trash, both temporary.
type world struct {
	t          *testing.T
	home, bin  string
	m          Manifest
	at         time.Time
	sessionDir string
}

func newWorld(t *testing.T) *world {
	root := t.TempDir()
	w := &world{t: t, home: filepath.Join(root, "claude"), bin: filepath.Join(root, "Trash"),
		m: Manifest{Path: filepath.Join(root, "state", "trashed.json")}, at: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	w.sessionDir = filepath.Join(w.home, "projects", "-work")
	return w
}

// write creates a file under the Claude Code directory with data.
func (w *world) write(rel, data string) string {
	w.t.Helper()
	path := filepath.Join(w.home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		w.t.Fatal(err)
	}
	return path
}

// trash moves paths to the trash as a numbered name, as Move does on macOS,
// and returns the record.
func (w *world) trash(id string, paths ...string) Record {
	w.t.Helper()
	r := Record{SessionID: id, Name: "plan", Cwd: "/work"}
	for _, p := range paths {
		to, err := moveInto(p, w.bin)
		if err != nil {
			w.t.Fatal(err)
		}
		r.Files = append(r.Files, File{From: p, To: to})
	}
	return r
}

func (w *world) add(command string, r Record) {
	w.t.Helper()
	if err := w.m.Add(w.at, command, r); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) entries() []Entry {
	w.t.Helper()
	entries, err := w.m.Entries()
	if err != nil {
		w.t.Fatal(err)
	}
	return entries
}

func TestRestorePutsASessionsFilesBackFromEveryPlace(t *testing.T) {
	w := newWorld(t)
	paths := []string{
		w.write("projects/-work/"+sid+".jsonl", "transcript"),
		filepath.Dir(w.write("projects/-work/"+sid+"/subagents/a.jsonl", "agent")),
		filepath.Dir(w.write("file-history/"+sid+"/f@v1", "v1")),
		filepath.Dir(w.write("session-env/"+sid+"/env", "env")),
		w.write("debug/"+sid+".txt", "debug"),
	}
	paths[1] = filepath.Dir(paths[1])
	w.add("session gc", w.trash(sid, paths...))

	entries := w.entries()
	if len(entries) != 1 || entries[0].ID() != "abcd1234" || entries[0].Size != int64(len("transcriptagentv1envdebug")) ||
		!entries[0].Trashed.Equal(w.at) || entries[0].Missing != 0 || len(entries[0].Files) != 5 {
		t.Fatalf("entries %+v", entries)
	}
	if entries[0].Files[0].Command != "session gc" || !entries[0].Files[0].InTrash {
		t.Fatalf("file %+v", entries[0].Files[0])
	}

	plan, problems := w.m.Plan([]string{"abcd1234"})
	if len(problems) != 0 || len(plan) != 5 {
		t.Fatalf("plan %v, problems %v", plan, problems)
	}
	res, err := w.m.Restore([]string{"abcd1234"})
	if err != nil || len(res.Back) != 5 || len(res.Failed) != 0 {
		t.Fatalf("restore %+v, err %v", res, err)
	}
	for _, p := range paths {
		if !exists(p) {
			t.Errorf("%s did not come back", p)
		}
	}
	if exists(w.m.Path) {
		t.Fatal("the manifest outlived its last record")
	}
}

func TestRestoreRefusesATakenPathAndMovesNothing(t *testing.T) {
	w := newWorld(t)
	transcript := w.write("projects/-work/"+sid+".jsonl", "old")
	debug := w.write("debug/"+sid+".txt", "debug")
	r := w.trash(sid, transcript, debug)
	w.add("session gc", r)
	w.write("projects/-work/"+sid+".jsonl", "new")

	_, err := w.m.Restore([]string{"abcd1234"})
	var blocked *Blocked
	if !errors.As(err, &blocked) || !strings.Contains(err.Error(), transcript+" exists") {
		t.Fatalf("err %v", err)
	}
	if data, _ := os.ReadFile(transcript); string(data) != "new" {
		t.Fatalf("the transcript there now was replaced with %q", data)
	}
	if exists(debug) || !exists(r.Files[1].To) {
		t.Fatal("a refused restore moved the debug log")
	}
	if len(w.entries()) != 1 {
		t.Fatal("a refused restore changed the manifest")
	}
}

func TestRestoreRefusesAFileNoLongerInTheTrash(t *testing.T) {
	w := newWorld(t)
	r := w.trash(sid, w.write("projects/-work/"+sid+".jsonl", "t"), w.write("debug/"+sid+".txt", "d"))
	w.add("session gc", r)
	if err := os.Remove(r.Files[0].To); err != nil {
		t.Fatal(err)
	}

	if e := w.entries(); len(e) != 1 || e[0].Missing != 1 || e[0].Size != 1 {
		t.Fatalf("entries %+v", e)
	}
	_, err := w.m.Restore([]string{"abcd1234"})
	if err == nil || !strings.Contains(err.Error(), r.Files[0].From+" is no longer in the trash") {
		t.Fatalf("err %v", err)
	}
	if exists(r.Files[1].From) {
		t.Fatal("restored the debug log without its transcript")
	}

	// Once the trash is emptied, the record goes too.
	if err := os.Remove(r.Files[1].To); err != nil {
		t.Fatal(err)
	}
	if e := w.entries(); len(e) != 0 {
		t.Fatalf("entries of an emptied trash %+v", e)
	}
	w.add("session gc", w.trash("ffff0000-1111", w.write("debug/ffff0000-1111.txt", "d")))
	data, err := os.ReadFile(w.m.Path)
	if err != nil || strings.Contains(string(data), sid) {
		t.Fatalf("the next write kept the emptied record:\n%s", data)
	}
}

func TestAPartialRestoreKeepsWhatStayedInTheTrash(t *testing.T) {
	w := newWorld(t)
	transcript := w.write("projects/-work/"+sid+".jsonl", "t")
	history := filepath.Dir(w.write("file-history/"+sid+"/f@v1", "v1"))
	r := w.trash(sid, transcript, history)
	r.Moves = json.RawMessage(`{"-work/` + sid + `":"/work"}`)
	w.add("session gc", r)
	// A file where file-history should be keeps its directory from coming back.
	if err := os.RemoveAll(filepath.Join(w.home, "file-history")); err != nil {
		t.Fatal(err)
	}
	w.write("file-history", "in the way")

	res, err := w.m.Restore([]string{"abcd1234"})
	if err != nil || len(res.Back) != 1 || res.Back[0].From != transcript || len(res.Failed) != 1 || res.Failed[0].From != history {
		t.Fatalf("restore %+v, err %v", res, err)
	}
	if res.Moves != nil {
		t.Fatalf("handed back the moves before every file was back: %s", res.Moves)
	}
	e := w.entries()
	if len(e) != 1 || len(e[0].Files) != 1 || e[0].Files[0].From != history {
		t.Fatalf("entries after a partial restore %+v", e)
	}

	if err := os.Remove(filepath.Join(w.home, "file-history")); err != nil {
		t.Fatal(err)
	}
	res, err = w.m.Restore([]string{"abcd1234"})
	if err != nil || len(res.Back) != 1 || len(res.Moves) != 1 || !strings.Contains(string(res.Moves[0]), `"-work/`+sid+`": "/work"`) {
		t.Fatalf("retry %+v, err %v", res, err)
	}
	if !exists(history) || exists(w.m.Path) {
		t.Fatal("the retry did not finish the restore")
	}
}

func TestRestorePutsADirectoryBackBeforeWhatGoesInIt(t *testing.T) {
	w := newWorld(t)
	sidecar := filepath.Join(w.sessionDir, sid)
	outputs := filepath.Dir(w.write("projects/-work/"+sid+"/tool-results/r.txt", "r"))
	w.add("session gc", w.trash(sid, outputs))
	w.write("projects/-work/"+sid+"/subagents/a.jsonl", "a")
	w.at = w.at.Add(time.Hour)
	w.add("session gc", w.trash(sid, w.write("projects/-work/"+sid+".jsonl", "t"), sidecar))

	if e := w.entries(); len(e) != 1 || len(e[0].Files) != 3 || !e[0].Trashed.Equal(w.at) {
		t.Fatalf("entries %+v", e)
	}
	if res, err := w.m.Restore([]string{"abcd1234"}); err != nil || len(res.Failed) != 0 {
		t.Fatalf("restore %+v, err %v", res, err)
	}
	for _, p := range []string{filepath.Join(outputs, "r.txt"), filepath.Join(sidecar, "subagents", "a.jsonl")} {
		if !exists(p) {
			t.Errorf("%s did not come back", p)
		}
	}
}

func TestATeamDirectoryJoinsItsSessionsEntry(t *testing.T) {
	w := newWorld(t)
	w.add("gc --teams", w.trash("abcd1234", filepath.Dir(w.write("teams/session-abcd1234/config.json", "{}"))))
	w.at = w.at.Add(time.Hour)
	w.add("session gc", w.trash(sid, w.write("projects/-work/"+sid+".jsonl", "t")))

	e := w.entries()
	if len(e) != 1 || e[0].SessionID != sid || len(e[0].Files) != 2 || !slices.Contains(e[0].Aliases(), "abcd1234") {
		t.Fatalf("entries %+v", e)
	}
}

func TestAddRecordsNothingTheTrashDidNotPlace(t *testing.T) {
	w := newWorld(t)
	if err := w.m.Add(w.at, "session gc", Record{SessionID: sid, Files: []File{{From: "/x"}}}); err != nil {
		t.Fatal(err)
	}
	if err := (Manifest{}).Add(w.at, "session gc", w.trash(sid, w.write("debug/x.txt", "d"))); err != nil {
		t.Fatal(err)
	}
	if exists(w.m.Path) {
		t.Fatal("recorded a file with nowhere to put it back from")
	}
}

func TestRestoreRefusesWhatItHasNoRecordOf(t *testing.T) {
	w := newWorld(t)
	if _, err := w.m.Restore([]string{"abcd1234"}); err == nil || !strings.Contains(err.Error(), "no record of abcd1234") {
		t.Fatalf("err %v", err)
	}
}

func TestRestoreRefusesAPathTrashedTwice(t *testing.T) {
	w := newWorld(t)
	debug := w.write("debug/"+sid+".txt", "one")
	w.add("session gc", w.trash(sid, debug))
	w.write("debug/"+sid+".txt", "two")
	w.at = w.at.Add(time.Hour)
	w.add("session gc", w.trash(sid, debug))
	if _, err := w.m.Restore([]string{"abcd1234"}); err == nil || !strings.Contains(err.Error(), "went to the trash twice") {
		t.Fatalf("err %v", err)
	}
}

func TestAddsFromSeveralProcessesLoseNothing(t *testing.T) {
	w := newWorld(t)
	var records []Record
	for i := range 20 {
		id := fmt.Sprintf("%08x-0000", i)
		records = append(records, w.trash(id, w.write("debug/"+id+".txt", "d")))
	}
	var wg sync.WaitGroup
	for _, r := range records {
		wg.Go(func() {
			if err := w.m.Add(w.at, "session gc", r); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if e := w.entries(); len(e) != 20 {
		t.Fatalf("%d entries", len(e))
	}
}

func TestTwoRestoresAtOnceMoveEachFileOnce(t *testing.T) {
	w := newWorld(t)
	w.add("session gc", w.trash(sid, w.write("projects/-work/"+sid+".jsonl", "t"), w.write("debug/"+sid+".txt", "d")))
	var wg sync.WaitGroup
	results := make([]error, 2)
	backs := make([]int, 2)
	for i := range 2 {
		wg.Go(func() {
			res, err := w.m.Restore([]string{"abcd1234"})
			results[i], backs[i] = err, len(res.Back)
		})
	}
	wg.Wait()
	var blocked *Blocked
	ok := results[0] == nil && errors.As(results[1], &blocked) || results[1] == nil && errors.As(results[0], &blocked)
	if !ok || backs[0]+backs[1] != 2 {
		t.Fatalf("results %v, files back %v", results, backs)
	}
}

func TestAManifestClaudesCannotReadIsKept(t *testing.T) {
	w := newWorld(t)
	for _, data := range []string{"{oops", `{"version": 2, "runs": []}`} {
		if err := os.MkdirAll(filepath.Dir(w.m.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(w.m.Path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := w.m.Add(w.at, "session gc", w.trash(sid, w.write("debug/"+sid+".txt", "d"))); err == nil {
			t.Fatalf("added to %q", data)
		}
		if got, _ := os.ReadFile(w.m.Path); string(got) != data {
			t.Fatalf("manifest changed to %q", got)
		}
	}
}

func TestDefaultManifestIsInTheStateDirectory(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	m, err := DefaultManifest()
	if err != nil || m.Path != filepath.Join(state, "claudes", "trashed.json") {
		t.Fatalf("path %q, err %v", m.Path, err)
	}
}
