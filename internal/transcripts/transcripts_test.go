package transcripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestFolder(t *testing.T) {
	for cwd, want := range map[string]string{
		`C:\Users\me\src\claudes`: "C--Users-me-src-claudes",
		"/Users/me/my.repo":       "-Users-me-my-repo",
		"/home/me/über_x":         "-home-me--ber-x",
	} {
		if got := Folder(cwd); got != want {
			t.Errorf("Folder(%q) = %q, want %q", cwd, got, want)
		}
	}
}

// line is one transcript record.
func line(t *testing.T, fields map[string]any) string {
	t.Helper()
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func write(t *testing.T, path string, mtime time.Time, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

const sid = "ff78d735-9393-40d9-bcc8-af268e40b3b2"

func TestReadTakesTheNewestOfEachField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "-b", sid+".jsonl")
	at := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	write(t, path, at,
		line(t, map[string]any{"type": "custom-title", "customTitle": "old", "sessionId": sid}),
		line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a", "gitBranch": "main", "timestamp": "2026-11-14T10:00:00Z", "version": "2.1.300"}),
		line(t, map[string]any{"type": "ai-title", "aiTitle": "Fix the build", "sessionId": sid}),
		"{not json\n",
		line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/b", "gitBranch": "dev", "timestamp": "2026-11-14T11:00:00Z", "version": "2.1.325"}),
		line(t, map[string]any{"type": "last-prompt", "lastPrompt": "carry on", "sessionId": sid}),
		line(t, map[string]any{"type": "custom-title", "customTitle": "claudes-mvp", "sessionId": sid}),
		line(t, map[string]any{"type": "future-record", "unknown": true}),
	)
	s, err := summarize(path)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{s.SessionID, s.Name, s.Cwd, s.Branch, s.Version, s.LastPrompt, s.Path}
	want := []string{sid, "claudes-mvp", "/b", "dev", "2.1.325", "carry on", path}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("read (-want +got):\n%s", diff)
	}
	if !s.Started.Equal(time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)) || !s.LastActive.Equal(at) {
		t.Fatalf("started %v, last active %v", s.Started, s.LastActive)
	}
}

func TestReadFallsBackToAgentNameThenAITitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p", sid+".jsonl")
	write(t, path, time.Now(),
		line(t, map[string]any{"type": "ai-title", "aiTitle": "Fix the build", "sessionId": sid}),
		line(t, map[string]any{"type": "agent-name", "agentName": "builder", "sessionId": sid}),
	)
	if s, _ := summarize(path); s.Name != "builder" {
		t.Fatalf("name %q, want the agent name", s.Name)
	}
}

func TestReadScansTheWholeFileWhenBothEndsLackTheFolder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p", sid+".jsonl")
	blob := line(t, map[string]any{"type": "api-request-blob", "data": strings.Repeat("x", headSize+tailSize)})
	write(t, path, time.Now(),
		blob,
		line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/middle", "timestamp": "2026-11-14T10:00:00Z"}),
		blob,
	)
	s, err := summarize(path)
	if err != nil || s.Cwd != "/middle" || s.SessionID != sid {
		t.Fatalf("session %+v, err %v", s, err)
	}
}

func TestListKeepsTheNewestCopyOfAMovedSession(t *testing.T) {
	dir := t.TempDir()
	old := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	rec := func(cwd string) string {
		return line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": cwd, "timestamp": "2026-11-01T00:00:00Z"})
	}
	write(t, filepath.Join(dir, "-a", sid+".jsonl"), old, rec("/a"))
	write(t, filepath.Join(dir, "-b", sid+".jsonl"), old.Add(time.Hour), rec("/b"))
	other := "0f000000-0000-0000-0000-000000000000"
	write(t, filepath.Join(dir, "-c", other+".jsonl"), old.Add(2*time.Hour),
		line(t, map[string]any{"type": "user", "sessionId": other, "cwd": "/c"}))
	write(t, filepath.Join(dir, "-c", other, "subagents", "agent-1.jsonl"), old, rec("/c"))

	list, err := Store{Dir: dir}.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].SessionID != other || list[1].Cwd != "/b" || len(list[1].Copies) != 1 {
		t.Fatalf("list %+v", list)
	}
	if !strings.HasSuffix(list[1].Copies[0], filepath.Join("-a", sid+".jsonl")) {
		t.Fatalf("copies %q", list[1].Copies)
	}
}

func TestListOfAMissingFolderIsEmpty(t *testing.T) {
	list, err := Store{Dir: filepath.Join(t.TempDir(), "none")}.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("list %v, err %v", list, err)
	}
}

func TestCopyCopiesTheTranscriptAndItsSidecar(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, at, line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"}))
	write(t, filepath.Join(dir, "-a", sid, "tool-results", "r.txt"), at, "result")
	st := Store{Dir: dir}
	s, err := summarize(src)
	if err != nil {
		t.Fatal(err)
	}

	dest, err := st.Copy(s, "/b")
	if err != nil {
		t.Fatal(err)
	}
	if dest != filepath.Join(dir, "-b") {
		t.Fatalf("dest %s", dest)
	}
	info, err := os.Stat(filepath.Join(dest, sid+".jsonl"))
	if err != nil || !info.ModTime().Equal(at) {
		t.Fatalf("copy %v, err %v", info, err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, sid, "tool-results", "r.txt")); err != nil || string(data) != "result" {
		t.Fatalf("sidecar %q, err %v", data, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("the original is gone: %v", err)
	}
	if _, err := st.Copy(s, "/a"); err == nil || !strings.Contains(err.Error(), "already lives under") {
		t.Fatalf("copy into its own directory: %v", err)
	}
}

func TestFolderNamesMatch(t *testing.T) {
	at := func(folder, cwd string) Session {
		return Session{Cwd: cwd, Path: filepath.Join("/p", folder, sid+".jsonl")}
	}
	if !FolderNamesMatch(nil) {
		t.Error("no sessions should not count against the naming")
	}
	if !FolderNamesMatch([]Session{at("-moved", "/old"), at("-a-b", "/a/b")}) {
		t.Error("one match among the newest should be enough")
	}
	var renamed []Session
	for range 12 {
		renamed = append(renamed, at("a_b", "/a/b"))
	}
	renamed = append(renamed, at("-a-b", "/a/b"))
	if FolderNamesMatch(renamed) {
		t.Error("ten new-style misses should mean the naming changed")
	}
}

func TestTheFolderIsTheOneThatNamesTheProjectFolder(t *testing.T) {
	dir := t.TempDir()
	rec := func(cwd string) string {
		return line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": cwd})
	}
	drifted := filepath.Join(dir, "-proj", sid+".jsonl")
	write(t, drifted, time.Now(), rec("/proj"), rec("/proj/sub"))
	if s, _ := summarize(drifted); s.Cwd != "/proj" {
		t.Fatalf("a session that wandered into a subfolder resumes in %q", s.Cwd)
	}
	relocated := filepath.Join(dir, "-old", sid+".jsonl")
	write(t, relocated, time.Now(), rec("/old"), line(t, map[string]any{"type": "relocated", "relocatedCwd": "/new"}))
	if s, _ := summarize(relocated); s.Cwd != "/new" {
		t.Fatalf("relocated session at %q", s.Cwd)
	}
}

func TestFolderCountsUTF16UnitsLikeClaudeCode(t *testing.T) {
	if got := Folder("/p/e\U0001F600x"); got != "-p-e--x" {
		t.Fatalf("got %q", got)
	}
}

func TestCopyRefusesToOverwriteAnotherCopyOrALongFolder(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, time.Now(), line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"}))
	existing := filepath.Join(dir, "-b", sid+".jsonl")
	write(t, existing, time.Now(), "only here\n")
	st := Store{Dir: dir}
	s, err := summarize(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Copy(s, "/b"); err == nil || !strings.Contains(err.Error(), "already has a copy") {
		t.Fatalf("err %v", err)
	}
	if data, _ := os.ReadFile(existing); string(data) != "only here\n" {
		t.Fatalf("the other copy changed: %q", data)
	}
	if _, err := st.Copy(s, "/"+strings.Repeat("x", 220)); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("long directory: %v", err)
	}
}

func TestListSkipsATranscriptWhoseIDIsNotAUUID(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "-a", "x&echo.jsonl"), time.Now(), line(t, map[string]any{"type": "user", "cwd": "/a"}))
	if list, err := (Store{Dir: dir}).List(); err != nil || len(list) != 0 {
		t.Fatalf("list %+v, err %v", list, err)
	}
}

func TestRemoveOriginalKeepsItWhenTheCopyDiffers(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, time.Now(), "original\n")
	dest := filepath.Join(dir, "-b")
	write(t, filepath.Join(dest, sid+".jsonl"), time.Now(), "something else\n")

	err := (Store{Dir: dir}).RemoveOriginal(Session{SessionID: sid, Path: src}, dest)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("the original went: %v", err)
	}
}

func TestRemoveOriginalKeepsItWhenTheSubagentCopyDiffers(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, time.Now(), "same\n")
	write(t, filepath.Join(strings.TrimSuffix(src, ".jsonl"), "subagents", "a.jsonl"), time.Now(), "full\n")
	dest := filepath.Join(dir, "-b")
	write(t, filepath.Join(dest, sid+".jsonl"), time.Now(), "same\n")
	write(t, filepath.Join(dest, sid, "subagents", "a.jsonl"), time.Now(), "part")

	err := (Store{Dir: dir}).RemoveOriginal(Session{SessionID: sid, Path: src}, dest)
	if err == nil || !strings.Contains(err.Error(), "subagent directory does not match") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSuffix(src, ".jsonl"), "subagents", "a.jsonl")); err != nil {
		t.Fatalf("the original subagent transcript went: %v", err)
	}
}

func TestACorruptMovesFileIsKept(t *testing.T) {
	dir := t.TempDir()
	moves := filepath.Join(t.TempDir(), "moves.json")
	write(t, moves, time.Now(), "{not json")
	src := filepath.Join(dir, "-a", sid+".jsonl")
	write(t, src, time.Now(), line(t, map[string]any{"type": "user", "sessionId": sid, "cwd": "/a"}))
	st := Store{Dir: dir, Moves: moves}
	s, err := summarize(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Copy(s, "/b"); err == nil || !strings.Contains(err.Error(), "not a valid moves file") {
		t.Fatalf("err %v", err)
	}
	if data, _ := os.ReadFile(moves); string(data) != "{not json" {
		t.Fatalf("the moves file was replaced: %q", data)
	}
	if _, err := st.List(); err != nil {
		t.Fatalf("list: %v", err)
	}
}
