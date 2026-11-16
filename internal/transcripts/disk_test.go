package transcripts

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUsageAddsUpEverythingASessionKeeps(t *testing.T) {
	root := t.TempDir()
	st := Store{Dir: filepath.Join(root, "projects"), Home: root}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	rec := line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"})
	write(t, filepath.Join(st.Dir, "-a", sid+".jsonl"), at, rec)
	write(t, filepath.Join(st.Dir, "-b", sid+".jsonl"), at.Add(time.Hour), rec)
	write(t, filepath.Join(st.Dir, "-a", sid, "tool-results", "r.txt"), at, strings.Repeat("a", 100))
	write(t, filepath.Join(st.Dir, "-b", sid, "subagents", "agent-1.jsonl"), at, strings.Repeat("b", 1000))
	write(t, filepath.Join(root, "file-history", sid, "f@v1"), at, strings.Repeat("c", 10))

	list, err := st.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v, err %v", list, err)
	}
	got := st.Usage(list[0])
	n := int64(len(rec))
	want := Disk{Transcript: n, Sidecar: 1000, Copies: n + 100, FileHistory: 10, Total: 2*n + 1110}
	if got != want {
		t.Fatalf("usage %+v, want %+v", got, want)
	}
	if m := st.Measure(list); len(m) != 1 || m[0] != want {
		t.Fatalf("measure %+v", m)
	}
}

func TestUsageNeverFollowsALink(t *testing.T) {
	root := t.TempDir()
	st := Store{Dir: filepath.Join(root, "projects")}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(st.Dir, "-a", sid+".jsonl")
	write(t, path, at, line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"}))
	big := filepath.Join(root, "big")
	write(t, filepath.Join(big, "data"), at, strings.Repeat("x", 5000))
	if err := os.Symlink(big, filepath.Join(st.Dir, "-a", sid)); err != nil {
		t.Skip("cannot make a link:", err)
	}
	s, err := summarize(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := st.Usage(s); d.Sidecar != 0 {
		t.Fatalf("followed the link: %+v", d)
	}
}

func TestLeftoversAreSessionFoldersWithNoTranscript(t *testing.T) {
	root := t.TempDir()
	st := Store{Dir: filepath.Join(root, "projects"), Home: root}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	kept := "0f000000-0000-0000-0000-000000000000"
	gone := "0e000000-0000-0000-0000-000000000000"
	write(t, filepath.Join(st.Dir, "-a", kept+".jsonl"), at, line(t, map[string]any{"type": "user", "sessionId": kept, "cwd": "/a"}))
	write(t, filepath.Join(st.Dir, "-a", kept, "tool-results", "r.txt"), at, "kept")
	write(t, filepath.Join(st.Dir, "-a", gone, "tool-results", "r.txt"), at, "gone!")
	write(t, filepath.Join(st.Dir, "-a", "memory", "MEMORY.md"), at, "notes")
	write(t, filepath.Join(st.Dir, "-a", "not-a-session", "x"), at, "x")
	write(t, filepath.Join(root, "file-history", kept, "f@v1"), at, "v1")
	write(t, filepath.Join(root, "file-history", gone, "f@v1"), at, "v1")

	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.Leftovers(list)
	if err != nil {
		t.Fatal(err)
	}
	want := []Leftover{
		{Path: filepath.Join(root, "file-history", gone), SessionID: gone, Kind: "file-history", Size: 2},
		{Path: filepath.Join(st.Dir, "-a", gone), SessionID: gone, Kind: "sidecar", Size: 5},
	}
	if len(got) != len(want) {
		t.Fatalf("leftovers %+v", got)
	}
	for i := range want {
		if got[i].Modified.IsZero() {
			t.Fatalf("leftover %d has no change time", i)
		}
		got[i].Modified = time.Time{}
		if got[i] != want[i] {
			t.Fatalf("leftover %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestWithoutAHomeFileHistoryIsLeftOut(t *testing.T) {
	if dir := (Store{}).FileHistory(sid); dir != "" {
		t.Fatalf("file history %q", dir)
	}
	if dir := (Store{Home: "/h"}).FileHistory("../../etc"); dir != "" {
		t.Fatalf("file history for an invalid id %q", dir)
	}
}

func TestAddAddsEveryPart(t *testing.T) {
	d := Disk{Transcript: 1, Sidecar: 2, Copies: 3, FileHistory: 4, Total: 10}
	d.Add(Disk{Transcript: 10, Sidecar: 20, Copies: 30, FileHistory: 40, Total: 100})
	if want := (Disk{Transcript: 11, Sidecar: 22, Copies: 33, FileHistory: 44, Total: 110}); d != want {
		t.Fatalf("sum %+v, want %+v", d, want)
	}
}

func TestEmptyIsATranscriptWithNoConversation(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	empty := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, empty, at,
		line(t, map[string]any{"type": "system", "sessionId": sid, "cwd": "/a"}),
		line(t, map[string]any{"type": "last-prompt", "lastPrompt": "from before", "sessionId": sid}))
	talked := filepath.Join(dir, "-a", "0f000000-0000-0000-0000-000000000000.jsonl")
	write(t, talked, at, line(t, map[string]any{"type": "assistant", "sessionId": sid, "cwd": "/a"}))
	big := filepath.Join(dir, "-a", "0e000000-0000-0000-0000-000000000000.jsonl")
	write(t, big, at, line(t, map[string]any{"type": "system", "sessionId": sid, "cwd": "/a", "pad": strings.Repeat("x", headSize)}))

	for path, want := range map[string]bool{empty: true, talked: false, big: false} {
		s, err := summarize(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.Empty != want {
			t.Errorf("%s: Empty %v, want %v", filepath.Base(path), s.Empty, want)
		}
	}
}

func TestSessionFilesListsWhatExists(t *testing.T) {
	root := t.TempDir()
	st := Store{Dir: filepath.Join(root, "projects"), Home: root}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	rec := line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"})
	write(t, filepath.Join(st.Dir, "-a", sid+".jsonl"), at, rec)
	write(t, filepath.Join(st.Dir, "-b", sid+".jsonl"), at.Add(time.Hour), rec)
	write(t, filepath.Join(st.Dir, "-a", sid, "tool-results", "r.txt"), at, "r")
	write(t, filepath.Join(root, "file-history", sid, "f@v1"), at, "v1")
	write(t, filepath.Join(root, "debug", sid+".txt"), at, "log")

	list, err := st.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v, err %v", list, err)
	}
	want := []string{
		filepath.Join(st.Dir, "-b", sid+".jsonl"),
		filepath.Join(st.Dir, "-a", sid+".jsonl"),
		filepath.Join(st.Dir, "-a", sid),
		filepath.Join(root, "file-history", sid),
		filepath.Join(root, "debug", sid+".txt"),
	}
	if got := st.SessionFiles(list[0]); !slices.Equal(got, want) {
		t.Fatalf("files\n got %q\nwant %q", got, want)
	}
}

func TestForgetMoveDropsOnlyThatCopy(t *testing.T) {
	root := t.TempDir()
	st := Store{Dir: filepath.Join(root, "projects"), Moves: filepath.Join(root, "moves.json")}
	a, b := filepath.Join(st.Dir, "-a", sid+".jsonl"), filepath.Join(st.Dir, "-b", sid+".jsonl")
	if err := st.recordMove(a, sid, "/a"); err != nil {
		t.Fatal(err)
	}
	if err := st.recordMove(b, sid, "/b"); err != nil {
		t.Fatal(err)
	}
	if err := st.ForgetMove(a, sid); err != nil {
		t.Fatal(err)
	}
	moves, err := st.loadMoves()
	if err != nil || len(moves) != 1 || moves[moveKey(b, sid)] != "/b" {
		t.Fatalf("moves %v, err %v", moves, err)
	}
	if err := st.ForgetMove(a, sid); err != nil {
		t.Fatalf("forgetting twice: %v", err)
	}
}
