package transcripts

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var searchNow = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// searchSession writes a transcript of lines for a session last active ago
// before searchNow.
func searchSession(t *testing.T, id, name string, ago time.Duration, lines ...string) Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return Session{SessionID: id, Name: name, Path: path, Size: info.Size(), LastActive: searchNow.Add(-ago)}
}

func prompt(text string) string {
	return `{"type":"user","message":{"role":"user","content":"` + text + `"}}`
}

func reply(text string) string {
	return `{"message":{"role":"assistant","content":[{"type":"text","text":"` + text + `"}]},"type":"assistant"}`
}

func toolCall(name, input string) string {
	return `{"message":{"role":"assistant","content":[{"type":"tool_use","name":"` + name + `","input":` + input + `}]},"type":"assistant"}`
}

func toolResult(text string) string {
	return `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"` + text + `"}]}}`
}

// ids searches list and returns the matching session ids, best first.
func ids(t *testing.T, list []Session, q Query) []string {
	t.Helper()
	q.Now = searchNow
	results, err := Search(list, q)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range results {
		out = append(out, r.SessionID)
	}
	return out
}

func TestSearchLooksOnlyAtWhatTheUserAndClaudeWrote(t *testing.T) {
	for _, tc := range []struct {
		name, line, where string
	}{
		{"typed prompt", prompt("rotate the zebra keys"), "you"},
		{"prompt in blocks", `{"type":"user","message":{"content":[{"type":"text","text":"rotate the zebra keys"}]}}`, "you"},
		{"reply", reply("I rotated the zebra keys."), "claude"},
		{"shell command", toolCall("Bash", `{"command":"zebra rotate --all","description":"Rotate"}`), "tool"},
		{"file path", toolCall("Read", `{"file_path":"/src/zebra.go"}`), "tool"},
		{"pattern", toolCall("Grep", `{"pattern":"zebra","path":"/src"}`), "tool"},
		{"tool output", toolResult("zebra: 3 keys rotated"), ""},
		{"text a tool writes", toolCall("Write", `{"file_path":"/src/a.go","content":"zebra"}`), ""},
		{"meta record", `{"type":"user","isMeta":true,"message":{"content":"zebra caveat"}}`, ""},
		{"compaction summary", `{"type":"user","isCompactSummary":true,"message":{"content":"The zebra work so far"}}`, ""},
		{"reminder", prompt("<system-reminder>zebra</system-reminder>"), ""},
		{"slash command", prompt("<command-name>/zebra</command-name>"), ""},
		{"shell input", prompt("<bash-input>zebra</bash-input>"), ""},
		{"teammate message", prompt(`<teammate-message teammate_id=\"lead\">zebra</teammate-message>`), ""},
		{"interruption", prompt("[Request interrupted by user] zebra"), ""},
		{"subagent record", `{"type":"user","isSidechain":true,"message":{"content":"zebra task"}}`, ""},
		{"system record", `{"type":"system","subtype":"info","content":"zebra"}`, ""},
		{"attachment", `{"attachment":{"content":"zebra"},"type":"attachment"}`, ""},
		{"other record", `{"type":"api-request","body":{"messages":[{"role":"user","content":"zebra"}]}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := searchSession(t, "s1", "", time.Hour, tc.line)
			results, err := Search([]Session{s}, Query{Words: []string{"ZEBRA"}, Now: searchNow})
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tc.where == "" && len(results) > 0:
				t.Fatalf("matched %+v", results[0].Matches)
			case tc.where != "" && len(results) != 1:
				t.Fatal("no match")
			case tc.where != "" && results[0].Matches[0].Where != tc.where:
				t.Fatalf("matched in %q, want %q", results[0].Matches[0].Where, tc.where)
			}
		})
	}
}

func TestSearchToolsAddsToolOutput(t *testing.T) {
	list := []Session{
		searchSession(t, "string", "", time.Hour, toolResult("zebra: 3 keys rotated")),
		searchSession(t, "blocks", "", time.Hour, `{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"zebra"}]}]}}`),
	}
	if got := ids(t, list, Query{Words: []string{"zebra"}, Tools: true}); len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestSearchMatchesTheName(t *testing.T) {
	s := searchSession(t, "s1", "Zebra keys", time.Hour, prompt("hello"))
	results, err := Search([]Session{s}, Query{Words: []string{"zebra"}, Now: searchNow})
	if err != nil || len(results) != 1 || results[0].Matches[0] != (Match{Where: "name", Snippet: "Zebra keys"}) {
		t.Fatalf("%+v, %v", results, err)
	}
	if got := ids(t, []Session{s}, Query{Words: []string{"zebra"}, Who: "you"}); len(got) != 0 {
		t.Fatalf("--who you matched the name: %v", got)
	}
}

func TestSearchNeedsEveryWordInTheSession(t *testing.T) {
	both := searchSession(t, "both", "", time.Hour, prompt("feed the zebra"), reply("Fed it some hay."))
	one := searchSession(t, "one", "", time.Hour, prompt("feed the zebra"))
	if got := ids(t, []Session{both, one}, Query{Words: []string{"zebra", "HAY"}}); !slices.Equal(got, []string{"both"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSearchFindsTextJSONEscapes(t *testing.T) {
	s := searchSession(t, "s1", "", time.Hour,
		prompt(`say \"hi\" to C:\\work, then mention the Café on line\nTWO\tthree\u2028four\u0007five`))
	for _, word := range []string{`say "hi"`, `c:\work`, "café", "CAFÉ", "line two", "LINE", "two", "three", "four", "five"} {
		if got := ids(t, []Session{s}, Query{Words: []string{word}}); len(got) != 1 {
			t.Errorf("%q: no match", word)
		}
	}
	if got := ids(t, []Session{s}, Query{Words: []string{"cafe"}}); len(got) != 0 {
		t.Errorf("cafe matched café")
	}
}

func TestSearchReadsLinesLongerThanItsBuffer(t *testing.T) {
	long := strings.Repeat("plain words ", chunkSize/4)
	s := searchSession(t, "s1", "", time.Hour, prompt(long), prompt(long+" zebra"), reply("the giraffe"))
	if got := ids(t, []Session{s}, Query{Words: []string{"zebra", "giraffe"}}); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestSearchWho(t *testing.T) {
	asked := searchSession(t, "asked", "", time.Hour, prompt("zebra"))
	answered := searchSession(t, "answered", "", time.Hour, reply("zebra"))
	ran := searchSession(t, "ran", "", time.Hour, toolCall("Bash", `{"command":"zebra"}`))
	list := []Session{asked, answered, ran}
	if got := ids(t, list, Query{Words: []string{"zebra"}, Who: "you"}); !slices.Equal(got, []string{"asked"}) {
		t.Errorf("you: %v", got)
	}
	if got := ids(t, list, Query{Words: []string{"zebra"}, Who: "claude"}); !slices.Equal(got, []string{"answered"}) {
		t.Errorf("claude: %v", got)
	}
	if _, err := Search(list, Query{Words: []string{"zebra"}, Who: "tool"}); err == nil {
		t.Error("took who tool")
	}
}

func TestSearchRanMatchesOnlyShellCommands(t *testing.T) {
	list := []Session{
		searchSession(t, "ran", "", time.Hour, toolCall("Bash", `{"command":"make deploy-zebra","description":"Deploy"}`)),
		searchSession(t, "said", "", time.Hour, prompt("make deploy-zebra please")),
		searchSession(t, "read", "", time.Hour, toolCall("Read", `{"file_path":"/src/make deploy-zebra"}`)),
	}
	if got := ids(t, list, Query{Ran: "Deploy-Zebra"}); !slices.Equal(got, []string{"ran"}) {
		t.Fatalf("got %v", got)
	}
	if got := ids(t, list, Query{Ran: "deploy-zebra", Words: []string{"nowhere"}}); len(got) != 0 {
		t.Fatalf("--ran with a word that matches nothing: %v", got)
	}
}

func TestSearchTouchedMatchesOnlyEditedFiles(t *testing.T) {
	list := []Session{
		searchSession(t, "edited", "", time.Hour, toolCall("Edit", `{"file_path":"/src/zebra/keys.go","old_string":"a","new_string":"b"}`)),
		searchSession(t, "windows", "", time.Hour, toolCall("Write", `{"file_path":"C:\\src\\zebra\\keys.go","content":"x"}`)),
		searchSession(t, "notebook", "", time.Hour, toolCall("NotebookEdit", `{"notebook_path":"/src/zebra/keys.go.ipynb"}`)),
		searchSession(t, "read", "", time.Hour, toolCall("Read", `{"file_path":"/src/zebra/keys.go"}`)),
	}
	got := ids(t, list, Query{Touched: `zebra\keys.go`})
	slices.Sort(got)
	if !slices.Equal(got, []string{"edited", "notebook", "windows"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSearchPattern(t *testing.T) {
	list := []Session{
		searchSession(t, "match", "", time.Hour, prompt("ticket ZB-1234 is done")),
		searchSession(t, "case", "", time.Hour, prompt("ticket zb-1234 is done")),
		searchSession(t, "none", "", time.Hour, prompt("ticket ZB-12 is done")),
	}
	if got := ids(t, list, Query{Pattern: `ZB-\d{4}\b`}); !slices.Equal(got, []string{"match"}) {
		t.Errorf("case-sensitive: %v", got)
	}
	if got := ids(t, list, Query{Pattern: `(?i)zb-\d{4}\b`}); len(got) != 2 {
		t.Errorf("(?i): %v", got)
	}
	// No literal every match must hold, so every line is read.
	if got := ids(t, list, Query{Pattern: `\d{4}|\bzz`}); len(got) != 2 {
		t.Errorf("alternation: %v", got)
	}
	if _, err := Search(list, Query{Pattern: `(?<=x)y`}); err == nil {
		t.Error("took lookbehind, which RE2 does not have")
	}
}

func TestSearchRanksPromptsAboveToolCalls(t *testing.T) {
	var calls []string
	for range 50 {
		calls = append(calls, toolCall("Bash", `{"command":"zebra --check"}`))
	}
	list := []Session{
		searchSession(t, "tools", "", time.Hour, calls...),
		searchSession(t, "reply", "", time.Hour, reply("zebra"), reply("zebra again")),
		searchSession(t, "prompt", "", time.Hour, prompt("zebra")),
		searchSession(t, "name", "zebra", time.Hour, prompt("hello")),
	}
	if got := ids(t, list, Query{Words: []string{"zebra"}}); !slices.Equal(got, []string{"name", "prompt", "reply", "tools"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSearchRanksMoreMatchesAndRecentSessionsHigher(t *testing.T) {
	list := []Session{
		searchSession(t, "once", "", time.Hour, prompt("zebra")),
		searchSession(t, "often", "", time.Hour, prompt("zebra"), prompt("zebra"), prompt("zebra")),
		searchSession(t, "old", "", 90*24*time.Hour, prompt("zebra")),
	}
	if got := ids(t, list, Query{Words: []string{"zebra"}}); !slices.Equal(got, []string{"often", "once", "old"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSearchSkipsWhatItCannotRead(t *testing.T) {
	gone := Session{SessionID: "gone", Path: filepath.Join(t.TempDir(), "gone.jsonl")}
	s := searchSession(t, "s1", "", time.Hour, prompt("zebra"), "not json zebra")
	if got := ids(t, []Session{gone, s}, Query{Words: []string{"zebra"}}); !slices.Equal(got, []string{"s1"}) {
		t.Fatalf("got %v", got)
	}
	if _, err := Search([]Session{s}, Query{Words: []string{"  "}}); err == nil {
		t.Fatal("searched for nothing")
	}
}

func TestSearchMatchesWordsAtTheirStart(t *testing.T) {
	for text, want := range map[string]bool{
		"take the lock":       true,
		"two locks":           true,
		"Locking it":          true,
		"LOCK":                true,
		"the fileLock field":  true,
		"see /src/lock.go":    true,
		"my_lock and (lock)":  true,
		"a block of code":     false,
		"the blocker":         false,
		"unlock it":           false,
		"clock2lock":          false,
		"lockfile and blocks": true,
	} {
		s := searchSession(t, "s1", "", time.Hour, prompt(text))
		if got := len(ids(t, []Session{s}, Query{Words: []string{"lock"}})) == 1; got != want {
			t.Errorf("lock in %q: matched %v, want %v", text, got, want)
		}
	}
	block := searchSession(t, "block", "", time.Hour, prompt("a block of code"))
	if got := ids(t, []Session{block}, Query{Pattern: "lock"}); len(got) != 1 {
		t.Error("-e lock did not match inside block")
	}
	phrase := searchSession(t, "phrase", "", time.Hour, prompt("the deadlock race"), prompt(`a lock\nrace`))
	results, err := Search([]Session{phrase}, Query{Words: []string{"lock race"}, Now: searchNow})
	if err != nil || len(results) != 1 || results[0].Matches[0].Snippet != "a lock race" {
		t.Fatalf("a phrase at a word start: %+v, %v", results, err)
	}
	for _, word := range []string{".go", "-v"} {
		s := searchSession(t, "s1", "", time.Hour, toolCall("Bash", `{"command":"go test -v ./ls.go"}`))
		if got := ids(t, []Session{s}, Query{Words: []string{word}}); len(got) != 1 {
			t.Errorf("%q, which starts with punctuation, did not match inside a word", word)
		}
	}
}

func TestSearchMatchesInsideTextWrittenWithoutSpaces(t *testing.T) {
	for _, tc := range []struct{ text, word string }{
		{"私は東京に行きました", "東京"},
		{"東京タワーに行く", "タワー"},
		{"ソウルの서울에서", "서울"},
		{"ฉันไปกรุงเทพเมื่อวาน", "กรุงเทพ"},
		{"東京Tokyo", "tokyo"},
		{`東京Tokyo`, "tokyo"},
	} {
		s := searchSession(t, "s1", "", time.Hour, prompt(tc.text))
		if got := ids(t, []Session{s}, Query{Words: []string{tc.word}}); len(got) != 1 {
			t.Errorf("%q did not match in %q", tc.word, tc.text)
		}
	}
}

func TestSearchMatchesWordStartsAfterOtherLetters(t *testing.T) {
	for text, want := range map[string]bool{
		"größelock":  false,
		"naïvelock":  false,
		"Ålock":      false,
		"daß lock":   true,
		"→lock":      true,
		"café·lock":  true,
		"über-lock":  true,
		"ünlockable": false,
	} {
		s := searchSession(t, "s1", "", time.Hour, prompt(text))
		if got := len(ids(t, []Session{s}, Query{Words: []string{"lock"}})) == 1; got != want {
			t.Errorf("lock in %q: matched %v, want %v", text, got, want)
		}
	}
}

func TestSearchMatchesWordStartsInTheName(t *testing.T) {
	for name, want := range map[string]bool{
		"fileLock tuning": true,
		"lock race":       true,
		"the deadlock":    false,
		"東京lock":          true,
	} {
		s := searchSession(t, "s1", name, time.Hour, prompt("nothing to see"))
		results, err := Search([]Session{s}, Query{Words: []string{"lock"}, Now: searchNow})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(results) == 1; got != want {
			t.Errorf("lock in the name %q: matched %v, want %v", name, got, want)
		}
	}
}

func TestSearchRanksWordsInOneRecordAboveScatteredOnes(t *testing.T) {
	list := []Session{
		searchSession(t, "scattered", "", time.Hour,
			prompt("take the lock"), prompt("take the lock again"), prompt("who wins the race"), prompt("the race is on")),
		searchSession(t, "apart", "", time.Hour, prompt("the lock can lose a race")),
		searchSession(t, "phrase", "", 48*time.Hour, prompt("there is a lock race in pins.json")),
	}
	if got := ids(t, list, Query{Words: []string{"lock", "race"}}); !slices.Equal(got, []string{"phrase", "apart", "scattered"}) {
		t.Fatalf("got %v", got)
	}
	// Closeness counts after the place: both words in a reply still beat
	// them scattered over prompts.
	list = []Session{
		searchSession(t, "prompts", "", time.Hour, prompt("take the lock"), prompt("who wins the race")),
		searchSession(t, "reply", "", time.Hour, reply("a lock race")),
	}
	if got := ids(t, list, Query{Words: []string{"lock", "race"}}); !slices.Equal(got, []string{"reply", "prompts"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSearchSnippetComesFromTheClosestRecord(t *testing.T) {
	s := searchSession(t, "s1", "", time.Hour,
		prompt("take the lock first"),
		reply("the race went fine"),
		prompt("now look at the lock and the race together"),
		prompt("and the lock race in pins.json"),
	)
	results, err := Search([]Session{s}, Query{Words: []string{"lock", "race"}, Now: searchNow})
	if err != nil || len(results) != 1 {
		t.Fatalf("%+v, %v", results, err)
	}
	ms := results[0].Matches
	if ms[0] != (Match{Where: "you", Snippet: "and the lock race in pins.json"}) || ms[1].Snippet != "now look at the lock and the race together" {
		t.Fatalf("matches %+v", ms)
	}
}

func TestSnippet(t *testing.T) {
	text := strings.Repeat("lead in words ", 10) + "the\n\nzebra   keys" + strings.Repeat(" and more words", 10)
	start := strings.Index(text, "zebra")
	got := snippet(text, start, start+len("zebra"))
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") || !strings.Contains(got, "the zebra keys and") {
		t.Fatalf("snippet %q", got)
	}
	if r := []rune(got); len(r) > snippetLength+2 || strings.Contains(got, "  ") {
		t.Fatalf("snippet %q is %d characters", got, len(r))
	}
	if words := strings.Fields(strings.Trim(got, "…")); words[0] != "words" && words[0] != "in" && words[0] != "lead" {
		t.Fatalf("snippet %q starts inside a word", got)
	}
	if got := snippet("zebra", 0, 5); got != "zebra" {
		t.Fatalf("whole text: %q", got)
	}
}

func TestNeedle(t *testing.T) {
	for _, tc := range []struct {
		literals []string
		cut      string
		want     string
	}{
		{[]string{"zebra"}, "", "zebra"},
		{[]string{`say "hello there" now`}, "", "hello there"},
		{[]string{"C:/work/zebra-keys"}, "/", "zebra-keys"},
		{[]string{"x", "Zebra"}, "", "zebra"},
		{[]string{"café au"}, "", "caf"},
		{[]string{"éé"}, "", ""},
		{nil, "", ""},
	} {
		n := newNeedle(tc.literals, tc.cut)
		got := ""
		if n != nil {
			got = string(n.lower)
		}
		if got != tc.want {
			t.Errorf("newNeedle(%q, %q) = %q, want %q", tc.literals, tc.cut, got, tc.want)
		}
	}
	n := newNeedle([]string{"Zebra"}, "")
	for line, want := range map[string]int{"a ZEBRA": 2, "zebrazebra": 0, "zebr": -1, "": -1, "a zEbRa b": 2, "zzzzebra": 3} {
		if got := n.index([]byte(line)); got != want {
			t.Errorf("index(%q) = %d, want %d", line, got, want)
		}
	}
}
