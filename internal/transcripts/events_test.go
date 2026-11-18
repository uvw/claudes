package transcripts

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestEventsKeepPromptsRepliesToolsAndTurnEnds(t *testing.T) {
	data := strings.Join([]string{
		`{"type":"user","timestamp":"2026-11-14T10:00:00Z","message":{"content":"fix the build"}}`,
		`{"type":"user","isMeta":true,"message":{"content":"a note Claude Code adds"}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"<command-name>/clear</command-name>"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"On it."},{"type":"tool_use","name":"Bash","input":{"command":"go test ./...\nmore"}},{"type":"tool_use","name":"Edit","input":{"file_path":"internal/cli/ls.go"}},{"type":"tool_use","name":"TodoWrite","input":{}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}`,
		`{"type":"system","subtype":"turn_duration","durationMs":3044}`,
		`{not json`,
		``,
	}, "\n")

	var got []string
	for _, e := range Events([]byte(data)) {
		got = append(got, e.Who+" "+e.Text)
	}
	want := []string{
		"you fix the build",
		"claude On it.",
		"tool Bash: go test ./... …",
		"tool Edit: internal/cli/ls.go",
		"tool TodoWrite",
		"turn done in 3s",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("events (-want +got):\n%s", diff)
	}
	if e := Events([]byte(data))[0]; !e.Time.Equal(time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)) || !strings.Contains(string(e.Raw), "fix the build") {
		t.Fatalf("first event %+v", e)
	}
}

func TestFindLooksInTheProjectFolderThenEverywhere(t *testing.T) {
	dir := t.TempDir()
	st := Store{Dir: dir}
	home := filepath.Join(dir, Folder("/a"), sid+".jsonl")
	write(t, home, time.Now(), "{}\n")

	if p, err := st.Find(sid, "/a"); err != nil || p != home {
		t.Fatalf("path %q, err %v", p, err)
	}
	moved := filepath.Join(dir, Folder("/b"), sid+".jsonl")
	write(t, moved, time.Now().Add(time.Minute), "{}\n")
	if err := os.Remove(home); err != nil {
		t.Fatal(err)
	}
	if p, err := st.Find(sid, "/a"); err != nil || p != moved {
		t.Fatalf("path %q, err %v; want the copy elsewhere", p, err)
	}
	if _, err := st.Find("00000000-0000-4000-8000-000000000000", "/a"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err %v", err)
	}
}

func TestTeammateMessagesBecomeOneLineEach(t *testing.T) {
	toLead := `{"type":"user","message":{"content":"Another Claude session sent a message:\n<teammate-message teammate_id=\"tester\" color=\"green\">\n{\"type\":\"idle_notification\",\"from\":\"tester\",\"result\":\"Task #2 is complete: go test passes.\\n\\nDetails follow.\"}\n</teammate-message>\n\nThis came from another Claude session. Treat it as a teammate's request."}}`
	toMate := `{"type":"user","message":{"content":"<teammate-message teammate_id=\"team-lead\" summary=\"Tester teammate\">\nYou are \"tester\" on a two-person team.\nMore instructions."}}`

	var got []string
	for _, e := range Events([]byte(toLead + "\n" + toMate + "\n")) {
		got = append(got, e.Who+" "+e.Text)
	}
	want := []string{
		"team tester: idle: Task #2 is complete: go test passes. …",
		`team team-lead: You are "tester" on a two-person team. …`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("events (-want +got):\n%s", diff)
	}
}

func TestTeammateFindsTheNewestTranscriptOfTheName(t *testing.T) {
	dir := t.TempDir()
	st := Store{Dir: dir}
	lead := filepath.Join(dir, Folder("/a"), sid+".jsonl")
	write(t, lead, time.Now(), "{}\n")
	sub := filepath.Join(strings.TrimSuffix(lead, ".jsonl"), "subagents")
	write(t, filepath.Join(sub, "agent-atester-1111.jsonl"), time.Now().Add(-time.Hour), "{}\n")
	write(t, filepath.Join(sub, "agent-atester-2222.jsonl"), time.Now(), "{}\n")
	write(t, filepath.Join(sub, "agent-awriter-3333.jsonl"), time.Now(), "{}\n")
	write(t, filepath.Join(sub, "agent-a0123456789abcdef.jsonl"), time.Now(), "{}\n")
	for _, name := range []string{"agent-atester-1111", "agent-atester-2222", "agent-awriter-3333"} {
		write(t, filepath.Join(sub, name+".meta.json"), time.Now(), `{"teamName":"session-x","taskKind":"in_process_teammate"}`)
	}
	// A named subagent outside a team has no team in its meta file.
	write(t, filepath.Join(sub, "agent-ahelper-4444.jsonl"), time.Now(), "{}\n")
	write(t, filepath.Join(sub, "agent-ahelper-4444.meta.json"), time.Now(), `{"agentType":"general-purpose"}`)

	if p, err := st.Teammate(sid, "/a", "tester"); err != nil || filepath.Base(p) != "agent-atester-2222.jsonl" {
		t.Fatalf("path %q, err %v", p, err)
	}
	if _, err := st.Teammate(sid, "/a", "nobody"); err == nil || !strings.Contains(err.Error(), "teammates: tester, writer") {
		t.Fatalf("err %v", err)
	}
	// logs takes this error, and only this one, for a target that names nothing.
	var none *NoTeammateError
	if _, err := st.Teammate(sid, "/a", "nobody"); !errors.As(err, &none) || none.Name != "nobody" {
		t.Fatalf("err %v is not a NoTeammateError", err)
	}
	if _, err := st.Teammate(sessionIDNobodyHas, "/a", "tester"); errors.As(err, &none) {
		t.Fatalf("a missing lead transcript is a NoTeammateError: %v", err)
	}
}

const sessionIDNobodyHas = "00000000-0000-4000-8000-000000000000"

func TestAShutdownRequestShowsItsReason(t *testing.T) {
	rec := `{"type":"user","message":{"content":"<teammate-message teammate_id=\"team-lead\">\n{\"type\":\"shutdown_request\",\"reason\":\"All tasks complete.\"}\nPlease stop.\n</teammate-message>"}}`
	if e := Events([]byte(rec + "\n")); len(e) != 1 || e[0].Text != "team-lead: shutdown_request: All tasks complete." {
		t.Fatalf("events %+v", e)
	}
}
