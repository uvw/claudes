package transcripts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// On Windows a project directory can exist in another case than Folder(dir)
// gives, as when Claude Code ran from a cwd typed in lower case. The copy lands
// in that directory, but moves.json keys it by the computed name, so List never
// finds the entry and the copy resumes in the old directory.
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
	if _, err := st.Copy(s, `C:\x\Repo`, nil); err != nil {
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
	dest, err := st.Copy(s, "/b", nil)
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

// A move with options is a record of its own in moves.json; one without stays
// the directory alone, so a moves file that holds only directories still reads.
func TestAMoveKeepsTheJobOptions(t *testing.T) {
	dir := t.TempDir()
	moves := filepath.Join(t.TempDir(), "moves.json")
	other := "bbbbbbbb-1111-2222-3333-444455556666"
	write(t, moves, time.Now(), `{"-x/`+other+`": "/x"}`)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, at, line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"}))
	write(t, filepath.Join(dir, "-x", other+".jsonl"), at, line(t, map[string]any{"type": "user", "sessionId": other, "cwd": "/a"}))
	st := Store{Dir: dir, Moves: moves}
	s, _ := summarize(src)
	if _, err := st.Copy(s, "/b", []string{"--model", "m"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(moves)
	want := `{
  "-b/` + sid + `": {
    "dir": "/b",
    "options": [
      "--model",
      "m"
    ]
  },
  "-x/` + other + `": "/x"
}
`
	if diff := cmp.Diff(want, string(data)); diff != "" {
		t.Fatalf("moves.json (-want +got):\n%s", diff)
	}
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, s := range list {
		got[s.Cwd] = s.JobOptions()
	}
	if diff := cmp.Diff(map[string][]string{"/b": {"--model", "m"}, "/x": nil}, got); diff != "" {
		t.Fatalf("cwd and options (-want +got):\n%s", diff)
	}
}
