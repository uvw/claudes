package transcripts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// On Windows a project folder can exist in another case than Folder(dir)
// gives, as when Claude Code ran from a cwd typed in lower case. The copy lands
// in that folder, but moves.json keys it by the computed name, so List never
// finds the entry and the copy resumes in the old folder.
func TestAMoveIntoAFolderInAnotherCaseIsFound(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive file system")
	}
	dir := t.TempDir()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, at, line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"}))
	if err := os.MkdirAll(filepath.Join(dir, "c--x-repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := Store{Dir: dir, Moves: filepath.Join(t.TempDir(), "moves.json")}
	s, err := summarize(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Copy(s, `C:\x\Repo`); err != nil {
		t.Fatal(err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Cwd != `C:\x\Repo` {
		t.Fatalf("listed %+v, want the copy in C:\\x\\Repo", list)
	}
}

// moves.json records where session mv put a copy. A later relocation that
// Claude Code writes into that copy is newer, and should win.
func TestARelocationAfterAMoveWins(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, at, line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"}))
	st := Store{Dir: dir, Moves: filepath.Join(t.TempDir(), "moves.json")}
	s, _ := summarize(src)
	dest, err := st.Copy(s, "/b")
	if err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dest, sid+".jsonl")
	data, _ := os.ReadFile(copied)
	write(t, copied, at.Add(time.Hour), string(data),
		line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/b"}),
		line(t, map[string]any{"type": "relocated", "relocatedCwd": "/c"}))
	list, _ := st.List()
	if len(list) != 1 || list[0].Cwd != "/c" {
		t.Fatalf("listed %+v", list)
	}
}
