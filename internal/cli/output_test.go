package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.yaml.in/yaml/v4"

	"github.com/uvw/claudes/internal/claude"
)

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

func TestUnknownOutputIsAUsageError(t *testing.T) {
	for _, args := range [][]string{{"ls", "-o", "xml"}, {"where", "alpha", "-o", "xml"}} {
		h := newHarness(t)
		h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
		h.fails(t, args...)
		assertContains(t, h.err.String(), "must be text, json, or yaml")
		if h.out.Len() != 0 {
			t.Fatalf("%v printed:\n%s", args, h.out)
		}
	}
}

func TestWhereOneMatchPrintsTheBareFolder(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a, h.bg("bravo", "idle")}}

	h.ok(t, "where", "alpha")

	if h.out.String() != a.Cwd+"\n" {
		t.Fatalf("output %q", h.out.String())
	}
}

func TestWherePatternPrintsEveryMatch(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("plan-a", "idle"), h.bg("plan-b", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b, h.bg("other", "idle")}}

	h.ok(t, "where", "plan*")

	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "plan-a") || !strings.Contains(lines[0], a.Cwd) ||
		!strings.Contains(lines[1], "plan-b") || !strings.Contains(lines[1], b.Cwd) {
		t.Fatalf("output:\n%s", h.out)
	}
}

func TestWhereJSON(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}

	h.ok(t, "where", "alpha", "-o", "json")

	fromWhere := h.out.String()
	h.out.Reset()
	h.ok(t, "ls", "alpha", "-o", "json")
	if fromWhere != h.out.String() {
		t.Fatalf("where:\n%s\nls:\n%s", fromWhere, h.out)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(fromWhere), &got); err != nil || len(got) != 1 {
		t.Fatalf("got %v, err %v", got, err)
	}
	if got[0]["sessionId"] != a.SessionID || got[0]["cwd"] != a.Cwd {
		t.Fatalf("got %v", got[0])
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

func TestWhereAndLsCompleteMoreThanOneName(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.bg("bravo", "idle")}}

	assertOffers(t, h.complete(t, "where", "alpha", ""), "bravo")
	assertOffers(t, h.complete(t, "ls", "alpha", ""), "bravo")
	assertOffers(t, h.complete(t, "ls", "-o", ""), "text", "json", "yaml")
}

func TestYAMLQuotesOnlyWhatNeedsIt(t *testing.T) {
	var buf strings.Builder
	type item struct {
		Name string `json:"name"`
		ID   string `json:"id"`
		Cwd  string `json:"cwd"`
	}
	err := (&outputFlag{"yaml"}).encode(&buf, []item{{Name: "alpha", ID: "12345678", Cwd: `C:\x`}})
	if err != nil {
		t.Fatal(err)
	}
	want := "- name: alpha\n  id: \"12345678\"\n  cwd: C:\\x\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
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
