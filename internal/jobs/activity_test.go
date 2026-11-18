package jobs

import (
	"fmt"
	"testing"
)

// show prints an Activity's fields, with nil as "-".
func show(a Activity) string {
	f := func(p any) string {
		switch v := p.(type) {
		case *int:
			if v != nil {
				return fmt.Sprint(*v)
			}
		case *int64:
			if v != nil {
				return fmt.Sprint(*v)
			}
		case *string:
			if v != nil {
				return *v
			}
		}
		return "-"
	}
	return fmt.Sprintf("subagents=%s teammates=%s shells=%s tokens=%s prs=%s doing=%s",
		f(a.Subagents), f(a.Teammates), f(a.Shells), f(a.Tokens), f(a.PRs), f(a.Doing))
}

func TestActivityReadsWhatRunsFromStateJSON(t *testing.T) {
	for name, tc := range map[string]struct{ state, want string }{
		"a session at work": {`{
			"state": "working", "detail": "running the tests", "tokens": 1400000,
			"inFlight": {"tasks": 4, "queued": 0, "kinds": ["local_bash", "local_agent", "in_process_teammate"]},
			"fan": [
				{"id": "b1234567a", "kind": "shell", "label": "go test", "startedAt": 1},
				{"id": "a0123456789abcdef", "kind": "agent", "label": "review", "startedAt": 2},
				{"id": "a1123456789abcdef", "kind": "agent", "label": "explore", "startedAt": 3, "doneAt": 0},
				{"id": "t9876543z", "kind": "agent", "label": "a teammate", "startedAt": 4, "doneAt": 0},
				{"id": "a2123456789abcdef", "kind": "agent", "label": "finished", "startedAt": 5, "doneAt": 6},
				{"id": "k1234567m", "kind": "mcp", "label": "a server", "startedAt": 7}
			],
			"children": [{"id": "1", "kind": "pr", "href": "https://example.com/1"}, {"id": "2", "kind": "pr"}, {"id": "3", "kind": "issue"}]
		}`, "subagents=2 teammates=1 shells=1 tokens=1400000 prs=2 doing=running the tests"},
		"nothing in flight leaves fan out": {
			`{"inFlight": {"tasks": 0, "queued": 0, "kinds": []}, "tokens": 30, "children": [], "detail": "done"}`,
			"subagents=0 teammates=0 shells=0 tokens=30 prs=0 doing=done"},
		"no fan and only a scheduled task": {
			`{"fan": null, "inFlight": {"tasks": 1, "kinds": ["session_cron"]}, "children": null}`,
			"subagents=0 teammates=0 shells=0 tokens=- prs=0 doing=-"},
		"no fan and other tasks in flight": {
			`{"inFlight": {"tasks": 2, "kinds": ["session_cron", "local_agent"]}}`,
			"subagents=- teammates=- shells=- tokens=- prs=- doing=-"},
		"no fan and tasks of no kind": {
			`{"inFlight": {"tasks": 1, "kinds": []}}`,
			"subagents=- teammates=- shells=- tokens=- prs=- doing=-"},
		"no fan and no inFlight": {`{"inFlight": null, "tokens": null}`, "subagents=- teammates=- shells=- tokens=- prs=- doing=-"},
		"odd types blank a field": {
			`{"fan": {"id": "x"}, "tokens": "1.4M", "children": 3, "detail": 7}`,
			"subagents=- teammates=- shells=- tokens=- prs=- doing=-"},
		"odd items are skipped": {
			`{"fan": ["shell", {"kind": "shell", "doneAt": "soon"}, {"kind": "shell"}], "tokens": 2.5, "children": ["pr", {"kind": "pr"}], "detail": "  "}`,
			"subagents=0 teammates=0 shells=1 tokens=- prs=1 doing=-"},
		"negative tokens":   {`{"tokens": -5}`, "subagents=- teammates=- shells=- tokens=- prs=- doing=-"},
		"not JSON":          {`{"fan": [`, "subagents=- teammates=- shells=- tokens=- prs=- doing=-"},
		"a JSON non-object": {`[1, 2]`, "subagents=- teammates=- shells=- tokens=- prs=- doing=-"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			writeJobFile(t, s, "abf00936", "state.json", tc.state)
			if got := show(s.Activity("abf00936")); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestActivityOfUnknownJobsIsBlank(t *testing.T) {
	s := newStore(t)
	writeJobFile(t, s, "", "state.json", `{"tokens": 5}`)
	want := "subagents=- teammates=- shells=- tokens=- prs=- doing=-"
	for _, id := range []string{"", "0b1d30b8", "..", "../jobs"} {
		if got := show(s.Activity(id)); got != want {
			t.Errorf("%q: %s", id, got)
		}
	}
}
