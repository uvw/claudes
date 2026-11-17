package transcripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadActivityCountsWhatTheUserDid(t *testing.T) {
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"<command-message>login</command-message>\n<command-name>/login</command-name>"}}`,
		`{"type":"user","message":{"role":"user","content":"<local-command-stdout>Login successful</local-command-stdout>"}}`,
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"<local-command-caveat>Caveat</local-command-caveat>"}}`,
		`{"type":"user","message":{"role":"user","content":"  fix the\n\tflaky   test  "}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"On it."},{"type":"tool_use","name":"Read","input":{}},{"type":"tool_use","name":"Edit","input":{}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"image","source":{}}]}}`,
		`{"type":"user","message":{"role":"user","content":"<task-notification>done</task-notification>"}}`,
		`{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"This session is being continued"}}`,
		`{"type":"user","message":{"role":"user","content":"<bash-input>ls</bash-input>"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{}}]}}`,
		`not json`,
	}
	got := activityOf(t, lines)
	want := Activity{Prompts: 2, Commands: 2, ToolCalls: 3, Edits: 2, FirstPrompt: "fix the flaky test"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadActivityTellsATeammateFromALeadThatGetsMessages(t *testing.T) {
	teammate := activityOf(t, []string{
		`{"type":"user","message":{"role":"user","content":"<teammate-message teammate_id=\"team-lead\">Take task 1</teammate-message>"}}`,
	})
	if !teammate.Teammate || teammate.Prompts != 0 {
		t.Errorf("a session the lead started: %+v", teammate)
	}
	lead := activityOf(t, []string{
		`{"type":"user","message":{"role":"user","content":"build it with a team"}}`,
		`{"type":"user","message":{"role":"user","content":"<teammate-message teammate_id=\"tester\">done</teammate-message>"}}`,
	})
	if lead.Teammate || lead.Prompts != 1 {
		t.Errorf("a lead: %+v", lead)
	}
}

func TestReadActivityCutsTheFirstPrompt(t *testing.T) {
	got := activityOf(t, []string{`{"type":"user","message":{"content":"` + strings.Repeat("é", 300) + `"}}`})
	if r := []rune(got.FirstPrompt); len(r) != 200 || r[199] != '…' {
		t.Errorf("first prompt is %d characters: %q", len(r), got.FirstPrompt)
	}
}

func activityOf(t *testing.T, lines []string) Activity {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := ReadActivity(path)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSummarizeSaysWhereTheNameCameFrom(t *testing.T) {
	for _, tc := range []struct{ records, name, source string }{
		{`{"type":"ai-title","aiTitle":"Fix tests"}` + "\n" + `{"type":"custom-title","customTitle":"mine"}`, "mine", "custom"},
		{`{"type":"ai-title","aiTitle":"Fix tests"}` + "\n" + `{"type":"agent-name","agentName":"docs"}`, "docs", "agent"},
		{`{"type":"ai-title","aiTitle":"Fix tests"}`, "Fix tests", "ai"},
		{`{"type":"user","cwd":"/w"}`, "", ""},
	} {
		path := filepath.Join(t.TempDir(), "s.jsonl")
		if err := os.WriteFile(path, []byte(tc.records+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := summarize(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != tc.name || s.NameSource != tc.source {
			t.Errorf("%s: got %q from %q, want %q from %q", tc.records, s.Name, s.NameSource, tc.name, tc.source)
		}
	}
}
