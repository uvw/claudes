package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/transcripts"
)

// said writes a transcript for a session named name, as transcript does, with
// lines after its first records. It returns the session id.
func (h *harness) said(t *testing.T, name, cwd string, ago time.Duration, lines ...string) string {
	t.Helper()
	id := h.transcript(t, name, cwd, ago)
	path := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(cwd), id+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	at := h.now.Add(-ago)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	return id
}

func typed(text string) string {
	return `{"type":"user","message":{"role":"user","content":"` + text + `"}}`
}

func answered(text string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + text + `"}]}}`
}

func ran(command string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"` + command + `"}}]}}`
}

func TestSessionSearchPrintsTheBestMatchesFirst(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	tool := h.said(t, "deploy-tool", work, time.Hour, ran("zebra-deploy --prod"))
	asked := h.said(t, "asked", work, 48*time.Hour, typed("please deploy the zebra service today"))
	h.said(t, "unrelated", work, time.Hour, typed("feed the giraffe"))

	h.ok(t, "session", "search", "Zebra")
	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if len(lines) < 3 || strings.Fields(lines[0])[0] != "ID" || !strings.HasSuffix(lines[0], "MATCH") {
		t.Fatalf("table:\n%s", h.out)
	}
	first, second := strings.Fields(lines[1]), strings.Fields(lines[2])
	if first[0] != asked[:8] || first[2] != "you" || !strings.HasSuffix(lines[1], "please deploy the zebra service today") {
		t.Fatalf("first row %q", lines[1])
	}
	if second[0] != tool[:8] || second[2] != "tool" || !strings.HasSuffix(lines[2], "Bash: zebra-deploy --prod") {
		t.Fatalf("second row %q", lines[2])
	}
	assertContains(t, h.out.String(), "Resume the first with:\n  claudes session resume "+asked[:8])
	if strings.Contains(h.out.String(), "unrelated") {
		t.Fatalf("listed a session that does not match:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "search", "okapi")
	if strings.TrimSpace(h.out.String()) != "No sessions match." {
		t.Fatalf("no match:\n%s", h.out)
	}
}

func TestSessionSearchJSON(t *testing.T) {
	h := newHarness(t)
	id := h.said(t, "zebra-keys", h.folder("work"), time.Hour, typed("rotate the zebra keys"), answered("Rotated."))

	h.ok(t, "session", "search", "zebra", "-o", "json")
	var hits []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &hits); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	if len(hits) != 1 {
		t.Fatalf("hits %v", hits)
	}
	keys := make([]string, 0, len(hits[0]))
	for k := range hits[0] {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if want := []string{"cwd", "id", "lastActive", "matches", "name", "score", "sessionId"}; !slices.Equal(keys, want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
	if hits[0]["sessionId"] != id || hits[0]["id"] != id[:8] || hits[0]["name"] != "zebra-keys" || hits[0]["score"].(float64) <= 0 {
		t.Fatalf("hit %v", hits[0])
	}
	matches := hits[0]["matches"].([]any)
	if len(matches) != 2 || matches[0].(map[string]any)["where"] != "name" ||
		matches[1].(map[string]any)["where"] != "you" || matches[1].(map[string]any)["snippet"] != "rotate the zebra keys" {
		t.Fatalf("matches %v", matches)
	}
	if strings.Contains(h.out.String(), "Resume") {
		t.Fatalf("JSON has the resume hint:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "search", "okapi", "-o", "json")
	if strings.TrimSpace(h.out.String()) != "[]" {
		t.Fatalf("no match: %s", h.out)
	}
}

func TestSessionSearchLimit(t *testing.T) {
	h := newHarness(t)
	for i, name := range []string{"one", "two", "three"} {
		h.said(t, name, h.folder(name), time.Duration(i+1)*time.Hour, typed("zebra"))
	}
	rows := func(args ...string) int {
		t.Helper()
		h.out.Reset()
		h.ok(t, append([]string{"session", "search", "zebra"}, args...)...)
		n := 0
		for line := range strings.SplitSeq(h.out.String(), "\n") {
			if strings.HasSuffix(line, " zebra") {
				n++
			}
		}
		return n
	}
	if n := rows("--limit", "2"); n != 2 {
		t.Fatalf("--limit 2 printed %d rows:\n%s", n, h.out)
	}
	assertContains(t, h.out.String(), "1 more matched; --limit 0 shows them all.")
	if n := rows("--limit", "0"); n != 3 || strings.Contains(h.out.String(), "more matched") {
		t.Fatalf("--limit 0 printed %d rows:\n%s", n, h.out)
	}
	if n := rows(); n != 3 {
		t.Fatalf("the default printed %d rows:\n%s", n, h.out)
	}
	h.fails(t, "session", "search", "zebra", "--limit", "-1")
}

func TestSessionSearchFilters(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	recent := h.said(t, "recent", work, time.Hour, typed("zebra"))
	old := h.said(t, "old", work, 10*24*time.Hour, typed("zebra"))
	elsewhere := h.said(t, "elsewhere", other, time.Hour, answered("zebra"))
	branched := h.said(t, "branched", other, 2*time.Hour, `{"type":"user","gitBranch":"zebra-branch","message":{"content":"zebra"}}`)
	shell := h.said(t, "shell", other, 3*time.Hour, ran("make zebra"))

	found := func(args ...string) []string {
		t.Helper()
		h.out.Reset()
		h.ok(t, append([]string{"session", "search", "-o", "json"}, args...)...)
		var hits []searchHit
		if err := json.Unmarshal(h.out.Bytes(), &hits); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, hit := range hits {
			ids = append(ids, hit.SessionID)
		}
		slices.Sort(ids)
		return ids
	}
	sorted := func(ids ...string) []string { slices.Sort(ids); return ids }
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"zebra", "-i", work}, sorted(recent, old)},
		{[]string{"zebra", "--in", work, "-l", "1d"}, sorted(recent)},
		{[]string{"zebra", "--since", "1d"}, sorted(recent, elsewhere, branched, shell)},
		{[]string{"zebra", "--branch", "zebra-branch"}, sorted(branched)},
		{[]string{"zebra", "--who", "claude"}, sorted(elsewhere)},
		{[]string{"zebra", "--who", "you", "-i", other}, sorted(branched)},
		{[]string{"--ran", "make zebra"}, sorted(shell)},
		{[]string{"-e", `^ze[b]ra$`, "--who", "you"}, sorted(recent, old, branched)},
		{[]string{"--regexp", `make\s+zebra`}, sorted(shell)},
	} {
		if got := found(tc.args...); !slices.Equal(got, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestSessionSearchTouched(t *testing.T) {
	h := newHarness(t)
	edited := h.said(t, "edited", h.folder("work"), time.Hour,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/src/zebra.go","old_string":"a","new_string":"b"}}]}}`)
	h.said(t, "read", h.folder("other"), time.Hour,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/src/zebra.go"}}]}}`)
	h.ok(t, "session", "search", "--touched", "zebra.go")
	assertContains(t, h.out.String(), edited[:8])
	if strings.Contains(h.out.String(), " read ") {
		t.Fatalf("--touched matched a file only read:\n%s", h.out)
	}
}

func TestSessionSearchTools(t *testing.T) {
	h := newHarness(t)
	h.said(t, "output", h.folder("work"), time.Hour, `{"type":"user","message":{"content":[{"type":"tool_result","content":"zebra found"}]}}`)
	h.ok(t, "session", "search", "zebra")
	assertContains(t, h.out.String(), "No sessions match.")
	h.out.Reset()
	h.ok(t, "session", "search", "zebra", "--tools")
	assertContains(t, h.out.String(), "output  zebra found")
}

func TestSessionSearchRefusesBadFlags(t *testing.T) {
	h := newHarness(t)
	h.said(t, "one", h.folder("work"), time.Hour, typed("zebra"))
	for _, args := range [][]string{
		{"session", "search"},
		{"session", "search", "zebra", "--who", "tool"},
		{"session", "search", "zebra", "--who", "you", "--tools"},
		{"session", "search", "-e", "(?<=a)b"},
		{"session", "search", "-e", "("},
	} {
		h.err.Reset()
		if code := h.run(args...); code == 0 {
			t.Errorf("%q succeeded", args)
		}
	}
	h.err.Reset()
	h.fails(t, "session", "search", "-e", "(")
	assertContains(t, h.err.String(), "-e: ")
}

func TestSessionSearchAnswersToGrep(t *testing.T) {
	h := newHarness(t)
	id := h.said(t, "one", h.folder("work"), time.Hour, typed("zebra"))
	h.ok(t, "s", "grep", "zebra")
	assertContains(t, h.out.String(), id[:8])
	if len(h.execs) != 0 {
		t.Fatalf("passed through to claude agents: %q", h.execs)
	}
}
