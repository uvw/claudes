package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/style"
)

// writeActivity writes a job's state.json with fan items running now: the
// subagents, teammates, and shells given, and one finished subagent.
func writeActivity(t *testing.T, h *harness, s claude.Session, subagents, teammates, shells int, extra string) {
	t.Helper()
	var fan []string
	for i := range subagents {
		fan = append(fan, fmt.Sprintf(`{"id":"a%016x","kind":"agent","label":"a subagent","startedAt":1,"doneAt":0}`, i))
	}
	for i := range teammates {
		fan = append(fan, fmt.Sprintf(`{"id":"t%08x","kind":"agent","label":"a teammate","startedAt":1}`, i))
	}
	for i := range shells {
		fan = append(fan, fmt.Sprintf(`{"id":"b%08x","kind":"shell","label":"a shell","startedAt":1}`, i))
	}
	fan = append(fan, `{"id":"affffffffffffffff","kind":"agent","label":"finished","startedAt":1,"doneAt":2}`)
	data := fmt.Sprintf(`{"cliVersion":"2.1.325","fan":[%s]%s}`, strings.Join(fan, ","), extra)
	writeFile(t, filepath.Join(h.app.Jobs.Dir, s.JobID, "state.json"), data)
}

// column is each session's cell under header in a text table, by name.
func column(t *testing.T, out, header string) map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	gap := regexp.MustCompile(`\s{2,}`)
	headers := gap.Split(lines[0], -1)
	at, name := -1, -1
	for i, h := range headers {
		switch h {
		case header:
			at = i
		case "NAME":
			name = i
		}
	}
	if at < 0 || name < 0 {
		t.Fatalf("no %s or NAME column:\n%s", header, out)
	}
	cells := map[string]string{}
	for _, line := range lines[1:] {
		row := gap.Split(strings.TrimRight(line, " "), -1)
		if len(row) != len(headers) {
			t.Fatalf("%q has %d cells, want %d:\n%s", line, len(row), len(headers), out)
		}
		cells[row[name]] = row[at]
	}
	return cells
}

func TestLsAgentsShowsTheTeamItLeadsAndTheSubagentsItRuns(t *testing.T) {
	h := newHarness(t)
	quiet, sub, lead, both := h.bg("quiet", "idle"), h.bg("sub", "busy"), h.bg("lead", "idle"), h.bg("both", "busy")
	term, gone := h.tty("term", "idle"), stopped(h.bg("gone", ""))
	h.agents.snapshots = [][]claude.Session{{quiet, sub, lead, both, term}}
	h.agents.finished = []claude.Session{gone}
	writeFile(t, filepath.Join(h.app.Jobs.Dir, quiet.JobID, "state.json"), `{"inFlight":{"tasks":0}}`)
	writeActivity(t, h, sub, 3, 1, 2, "")
	writeActivity(t, h, lead, 0, 2, 0, "")
	writeActivity(t, h, both, 3, 2, 0, "")
	writeActivity(t, h, gone, 4, 0, 1, "")
	h.team(t, lead, "writer", "tester")
	h.team(t, both, "writer", "tester")

	h.ok(t, "ls", "-a")
	got := column(t, h.out.String(), agentsHeader)
	want := map[string]string{"quiet": "-", "sub": "3", "lead": "lead+2", "both": "lead+2, 3", "term": "-", "gone": "-"}
	for name, cell := range want {
		if got[name] != cell {
			t.Errorf("%s: AGENTS is %q, want %q\n%s", name, got[name], cell, h.out)
		}
	}
	if strings.Contains(h.out.String(), "TEAM") || strings.Contains(h.out.String(), "SHELLS") {
		t.Errorf("ls shows a TEAM column or --wide's columns:\n%s", h.out)
	}
}

func TestLsWideAddsShellsTokensPRsAndWhatEachSessionIsDoing(t *testing.T) {
	h := newHarness(t)
	work, term := h.bg("work", "busy"), h.tty("term", "idle")
	h.agents.snapshots = [][]claude.Session{{work, term}}
	long := strings.Repeat("checking the build on every platform ", 4)
	writeActivity(t, h, work, 2, 0, 1, fmt.Sprintf(`,"tokens":1400000,"children":[{"id":"1","kind":"pr"},{"id":"2","kind":"pr"}],"detail":%q`, long))

	h.ok(t, "ls", "--wide")
	out := h.out.String()
	header := regexp.MustCompile(`(?m)^ID\s+PINNED\s+KIND\s+STATE\s+STATUS\s+AGENTS\s+SHELLS\s+TOKENS\s+PRS\s+AGE\s+VERSION\s+DIR\s+NAME\s+DOING$`)
	if !header.MatchString(out) {
		t.Fatalf("want SHELLS, TOKENS, and PRS before DIR, and DOING after NAME:\n%s", out)
	}
	for header, want := range map[string]map[string]string{
		"AGENTS": {"work": "2", "term": "-"},
		"SHELLS": {"work": "1", "term": "-"},
		"TOKENS": {"work": "1.4M", "term": "-"},
		"PRS":    {"work": "2", "term": "-"},
		"DOING":  {"work": strings.TrimSpace(long[:maxDoing-1]) + "…", "term": "-"},
	} {
		got := column(t, out, header)
		for name, cell := range want {
			if got[name] != cell {
				t.Errorf("%s: %s is %q, want %q\n%s", name, header, got[name], cell, out)
			}
		}
	}
}

func TestLsWideFitsDoingToTheTerminal(t *testing.T) {
	h := newHarness(t)
	work := h.bg("work", "busy")
	h.agents.snapshots = [][]claude.Session{{work}}
	writeActivity(t, h, work, 0, 0, 0, fmt.Sprintf(`,"detail":%q`, strings.Repeat("a long status line ", 20)))

	h.ok(t, "ls", "--wide")
	indent := strings.Index(h.out.String(), "DOING")
	for _, width := range []int{indent + 30, indent + 60} {
		h.app.Width = func() int { return width }
		h.out.Reset()
		h.ok(t, "ls", "--wide")
		lines := strings.Split(strings.TrimRight(h.out.String(), "\n"), "\n")
		// The cut drops a space before the …, so a line can end a column short.
		if got := style.Width(lines[1]); got > width || got < width-1 || !strings.HasSuffix(lines[1], "…") {
			t.Errorf("width %d: the line is %d columns:\n%s", width, got, h.out)
		}
	}

	h.app.Width = func() int { return 20 }
	h.out.Reset()
	h.ok(t, "ls", "--wide")
	if doing := column(t, h.out.String(), "DOING")["work"]; style.Width(doing) != minLast {
		t.Errorf("a narrow terminal cut DOING to %q, want %d columns", doing, minLast)
	}
}

func TestLsJSONHasTheCountsAndNullWhenUnknown(t *testing.T) {
	h := newHarness(t)
	work, term, gone := h.bg("work", "busy"), h.tty("term", "idle"), stopped(h.bg("gone", ""))
	h.agents.snapshots = [][]claude.Session{{work, term}}
	h.agents.finished = []claude.Session{gone}
	writeActivity(t, h, work, 2, 1, 1, `,"tokens":25080,"children":[{"kind":"pr"}],"detail":"writing docs"`)
	writeActivity(t, h, gone, 2, 0, 1, `,"tokens":"odd"`)

	h.ok(t, "ls", "-a", "-o", "json")
	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[fmt.Sprint(r["name"])] = fmt.Sprintf("%v %v %v %v %v", r["subagents"], r["shells"], r["tokens"], r["prs"], r["doing"])
	}
	for name, want := range map[string]string{
		"work": "2 1 25080 1 writing docs",
		"term": "<nil> <nil> <nil> <nil> <nil>",
		// A stopped session runs nothing, whatever its state.json lists.
		"gone": "0 0 <nil> <nil> <nil>",
	} {
		if got[name] != want {
			t.Errorf("%s: subagents, shells, tokens, prs, and doing are %q, want %q", name, got[name], want)
		}
	}
}

func TestTokenCountReadsAsAgentViewDoes(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0", 30: "30", 999: "999", 1000: "1.0K", 25_080: "25K", 171_170: "171K",
		999_600: "1.0M", 1_400_000: "1.4M", 3_956_924: "4.0M", 61_000_000: "61M", 2_500_000_000: "2.5B",
	} {
		if got := tokenCount(&n); got != want {
			t.Errorf("tokenCount(%d) = %q, want %q", n, got, want)
		}
	}
	if got := tokenCount(nil); got != "-" {
		t.Errorf("unknown tokens show %q", got)
	}
}

func TestCutEndsWhatItShortensWithAnEllipsis(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly 10", 10, "exactly 10"},
		{"one word too many", 10, "one word…"},
		{"日本語のテキスト", 7, "日本語…"},
	} {
		if got := cut(tc.in, tc.limit); got != tc.want || style.Width(got) > tc.limit {
			t.Errorf("cut(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
	}
}
