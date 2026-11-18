package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v4"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
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
	if doing := column(t, h.out.String(), "DOING")["work"]; style.Width(doing) != style.MinLast {
		t.Errorf("a narrow terminal cut DOING to %q, want %d columns", doing, style.MinLast)
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

func TestLsPutsPinnedFirstAndMarksThem(t *testing.T) {
	h := newHarness(t)
	idle, busy, pinned := h.bg("idle-one", "idle"), h.bg("busy-one", "busy"), h.bg("pinned-one", "idle")
	h.agents.snapshots = [][]claude.Session{{idle, busy, pinned}}
	h.pin(t, pinned)

	h.ok(t, "ls")

	lines := strings.Split(h.out.String(), "\n")
	if !strings.HasPrefix(lines[0], "ID ") || !strings.Contains(lines[1], " yes ") || !strings.HasSuffix(lines[1], "pinned-one") {
		t.Fatalf("pinned row is not first and marked:\n%s", h.out)
	}
	if !strings.Contains(lines[2], "busy-one") {
		t.Fatalf("busy row is not second:\n%s", h.out)
	}
	if !strings.Contains(lines[2], " - ") || strings.Contains(h.out.String(), "* pinned") {
		t.Fatalf("unpinned row or footer wrong:\n%s", h.out)
	}
}

func TestLsMarksSessionsBehindTheInstalledVersion(t *testing.T) {
	h := newHarness(t)
	old := h.bg("old-one", "idle")
	h.agents.snapshots = [][]claude.Session{{old}}
	writeState(t, h, old.JobID, "2.1.322")

	h.ok(t, "ls")

	assertContains(t, h.out.String(), "2.1.322 (old)")
}

func TestTheRunningExecutableOutranksAStaleCliVersion(t *testing.T) {
	h := newHarness(t)
	respawned, unknown := h.bg("respawned", "idle"), h.bg("npm", "idle")
	h.agents.snapshots = [][]claude.Session{{respawned, unknown}}
	// Claude Code keeps the old cliVersion until a respawned session takes a
	// turn; the executable it runs already names the new version.
	writeState(t, h, respawned.JobID, "2.1.322")
	writeState(t, h, unknown.JobID, "2.1.322")
	h.app.RunningVersion = func(pid int) string {
		if pid == respawned.PID {
			return "2.1.325"
		}
		return ""
	}

	h.ok(t, "ls")
	out := h.out.String()
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "respawned") && strings.Contains(line, "(old)"):
			t.Fatalf("a session running the installed version is marked old:\n%s", out)
		case strings.Contains(line, "npm") && !strings.Contains(line, "2.1.322 (old)"):
			t.Fatalf("a session with no version in its path lost its cliVersion:\n%s", out)
		}
	}

	h.ok(t, "restart", "--outdated")
	assertCalls(t, h, "respawn "+unknown.JobID)
}

func TestListingsShowNoMatchAsAnEmptyList(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	h.ok(t, "ls", "tour*")
	assertContains(t, h.out.String(), "No sessions.")
	h.out.Reset()
	h.ok(t, "ls", "tour*", "-o", "json")
	if got := strings.TrimSpace(h.out.String()); got != "[]" {
		t.Fatalf("json %q", got)
	}
	h.ok(t, "session", "ls", "tour*")
	h.exits(t, exit.NoMatch, "where", "tour*")
}

func TestLsShowsWhichSessionsLeadATeam(t *testing.T) {
	h := newHarness(t)
	lead, solo := h.bg("lead", "idle"), h.bg("solo", "idle")
	h.agents.snapshots = [][]claude.Session{{lead, solo}}
	h.team(t, lead, "writer", "tester")

	h.ok(t, "ls")
	assertContains(t, h.out.String(), "lead+2")
	h.out.Reset()
	h.ok(t, "ls", "lead", "-o", "json")
	var rows []struct{ Teammates []string }
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 1 || !slices.Equal(rows[0].Teammates, []string{"writer", "tester"}) {
		t.Fatalf("rows %+v, err %v\n%s", rows, err, h.out)
	}
}

func TestLsTextIsColoredWithCLICOLORFORCEAndLinesUpAsWithout(t *testing.T) {
	h := newHarness(t)
	lsWorld(t, h)
	h.ok(t, "ls", "-o", "text")
	plain := h.out.String()
	assertPlain(t, "ls without CLICOLOR_FORCE", plain)

	h.out.Reset()
	h.env("CLICOLOR_FORCE=1")
	h.ok(t, "ls", "-o", "text")

	out := h.out.String()
	for _, want := range []string{
		h.app.colors.Blue("yes"), h.app.colors.Green("busy"), h.app.colors.Magenta("waiting"),
		h.app.colors.Cyan("shell"), h.app.colors.Yellow("2.1.322 (old)"),
	} {
		assertContains(t, out, want)
	}
	if !strings.Contains(out, "\x1b[2m") {
		t.Fatalf("CWD is not faint:\n%q", out)
	}
	if strings.Contains(out, h.app.colors.Green("idle")) || !h.app.colors.Colors() {
		t.Fatalf("idle is colored, or colors are off:\n%q", out)
	}
	if got := sgr.ReplaceAllString(out, ""); got != plain {
		t.Fatalf("colored ls without its colors:\n%s\nwant:\n%s", got, plain)
	}
}

func TestLsNarrowsToTheTargets(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.bg("plan-a", "idle"), h.bg("plan-b", "idle")}}

	h.ok(t, "ls", "plan*")

	out := h.out.String()
	if !strings.Contains(out, "plan-a") || !strings.Contains(out, "plan-b") || strings.Contains(out, "alpha") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestLsPrintsASessionOnceWhenTwoTargetsMatchIt(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("plan-a", "idle")}}

	h.ok(t, "ls", "plan*", "plan-a", "-o", "json")

	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("rows %v, err %v", rows, err)
	}
}

func TestLsYAMLAndJSONHaveTheSameFields(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.tty("terminal", "busy")}}

	h.ok(t, "ls", "-o", "json")
	var fromJSON []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &fromJSON); err != nil {
		t.Fatal(err)
	}
	h.out.Reset()
	h.ok(t, "ls", "-o", "yaml")
	var fromYAML []map[string]any
	if err := yaml.Unmarshal(h.out.Bytes(), &fromYAML); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	if strings.Contains(h.out.String(), "{") {
		t.Fatalf("yaml is not in block style:\n%s", h.out)
	}
	if len(fromJSON) != 2 {
		t.Fatalf("json %v", fromJSON)
	}
	if diff := cmp.Diff(normalize(t, fromJSON), normalize(t, fromYAML)); diff != "" {
		t.Fatalf("yaml differs from json (-json +yaml):\n%s", diff)
	}
}

// normalize round-trips through JSON so YAML's ints compare equal to JSON's
// floats.
func normalize(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLsJSONFlagStillWorks(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
	h.ok(t, "ls", "--json")
	short := h.out.String()
	h.out.Reset()
	h.ok(t, "ls", "-o", "json")
	if short != h.out.String() || !strings.HasPrefix(short, "[") {
		t.Fatalf("--json:\n%s\n-o json:\n%s", short, h.out)
	}
}

func TestJSONHasEveryKeyForEverySession(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "idle")
	term.Name = ""
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "busy"), term}}
	h.agents.finished = []claude.Session{stopped(h.bg("gone", ""))}

	h.ok(t, "ls", "-a", "-o", "json")

	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 3 {
		t.Fatalf("rows %v, err %v", rows, err)
	}
	keys := []string{"id", "sessionId", "name", "cwd", "kind", "state", "status", "pid", "startedAt", "pinned", "version", "outdated", "teammates", "subagents", "shells", "tokens", "prs", "doing"}
	for _, r := range rows {
		if len(r) != len(keys) {
			t.Errorf("%v has %d keys, want %d", r, len(r), len(keys))
		}
		for _, k := range keys {
			if _, ok := r[k]; !ok {
				t.Errorf("%v has no %q", r, k)
			}
		}
	}
}

func TestLsShowsWhatSomeTargetsMatchAndNotesTheRest(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle"), h.bg("docs", "idle")}}

	h.ok(t, "ls", "api", "nothing", "-o", "json")
	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["name"] != "api" {
		t.Fatalf("rows %v, err %v\n%s", rows, err, h.out)
	}
	assertContains(t, h.err.String(), `no session matches "nothing"`)
}
