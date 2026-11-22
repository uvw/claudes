package transcripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTallyCountsWhatTheUserDid(t *testing.T) {
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
	want := Tally{Prompts: 2, Commands: 2, ToolCalls: 3, Edits: 2, FirstPrompt: "fix the flaky test", unsure: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadTallyLeavesOutSubagentRecords(t *testing.T) {
	got := activityOf(t, []string{
		`{"type":"user","message":{"role":"user","content":"ping"}}`,
		`{"type":"user","isSidechain":true,"message":{"role":"user","content":"look through the repository"}}`,
		`{"type":"assistant","isSidechain":true,"message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{}}]}}`,
	})
	if want := (Tally{Prompts: 1, FirstPrompt: "ping"}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadTallyIsCompleteOnlyWhenItKnowsEveryLine(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lines    []string
		complete bool
	}{
		{"known types and a blank line", []string{`{"type":"mode"}`, ``, `{"type":"user","message":{"content":"hi"}}`, `{"type":"api-request"}`}, true},
		{"a type claudes does not know", []string{`{"type":"user","message":{"content":"hi"}}`, `{"type":"assistant-turn","message":{}}`}, false},
		{"a line that does not parse", []string{`{"type":"user","message":{"content":"hi"}}`, `{"type":"assis`}, false},
		{"a record with no type", []string{`{"message":{"content":"hi"}}`}, false},
	} {
		if got := activityOf(t, tc.lines).Complete(); got != tc.complete {
			t.Errorf("%s: Complete %v, want %v", tc.name, got, tc.complete)
		}
	}
}

func TestReadTallyTellsATeammateFromALeadThatGetsMessages(t *testing.T) {
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

func TestReadTallyCutsTheFirstPrompt(t *testing.T) {
	got := activityOf(t, []string{`{"type":"user","message":{"content":"` + strings.Repeat("é", 300) + `"}}`})
	if r := []rune(got.FirstPrompt); len(r) != 200 || r[199] != '…' {
		t.Errorf("first prompt is %d characters: %q", len(r), got.FirstPrompt)
	}
}

func activityOf(t *testing.T, lines []string) Tally {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := ReadTally(path)
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

func TestReadTallyTellsHeadlessSessionsByTheirEntryPoint(t *testing.T) {
	for _, tc := range []struct {
		what    string
		entries []string
		want    bool
	}{
		{"claude -p", []string{"sdk-cli"}, true},
		{"the Agent SDK", []string{"sdk-ts", "sdk-ts"}, true},
		{"a terminal", []string{"cli"}, false},
		{"a terminal the Agent SDK drove later", []string{"cli", "sdk-ts"}, false},
		{"no entry point at all", nil, false},
	} {
		var lines []string
		for _, e := range tc.entries {
			lines = append(lines, `{"type":"user","entrypoint":"`+e+`","message":{"role":"user","content":"ping"}}`)
		}
		if len(lines) == 0 {
			lines = []string{`{"type":"user","message":{"role":"user","content":"ping"}}`}
		}
		if got := activityOf(t, lines).Headless; got != tc.want {
			t.Errorf("%s: headless %v, want %v", tc.what, got, tc.want)
		}
	}
}
