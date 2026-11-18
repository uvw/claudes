package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/transcripts"
)

func TestSessionTriageSortsSessionsIntoRemoveReviewAndKeep(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	week := 7 * 24 * time.Hour
	manyTools := []string{prompt("review the pull request")}
	for range reviewToolCalls + 1 {
		manyTools = append(manyTools, toolCall)
	}
	want := map[string]string{
		h.emptyTranscript(t, "e", work, week):                                                                         "remove empty",
		h.sessionOf(t, "ping", work, week, prompt("ping"), reply):                                                     "remove trivial",
		h.sessionOf(t, "lookup", work, week, prompt("which session ran the migration?"), toolCall):                    "review review",
		h.sessionOf(t, "skill", work, week, slashLogin, toolCall):                                                     "review review",
		h.sessionOf(t, "chat", work, week, prompt("hi"), reply, prompt("and?"), reply):                                "keep more than one prompt",
		h.sessionOf(t, "edit", work, week, prompt("fix it"), editCall):                                                "keep changed files",
		h.sessionOf(t, "named", work, week, prompt("look"), toolCall, `{"type":"custom-title","customTitle":"mine"}`): "keep named with /rename",
		h.sessionOf(t, "pr", work, week, manyTools...):                                                                "keep more than 20 tool calls",
		h.sessionOf(t, "today", work, time.Hour, prompt("ping")):                                                      "keep changed in the last day",
	}
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "triage", "-o", "json")
	var report triageReport
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for group, items := range map[string][]triageItem{"remove": report.Remove, "review": report.Review, "keep": report.Keep} {
		for _, it := range items {
			got[it.SessionID] = group + " " + it.Why
		}
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: got %q, want %q", id[:8], got[id], w)
		}
	}
	for _, it := range report.Review {
		if it.Activity == nil || it.Activity.FirstPrompt == "" && it.Activity.Commands == 0 {
			t.Errorf("review item %s has no prompt to judge: %+v", it.ID, it.Activity)
		}
	}
	if len(h.trashed) != 0 {
		t.Fatalf("triage trashed %q", h.trashed)
	}

	h.out.Reset()
	h.ok(t, "session", "triage")
	for _, line := range []string{"Remove 2 session(s)", "To remove them: claudes session gc --empty --trivial", "Review 2 session(s)",
		"which session ran the migration?", "Keep 5 session(s):", "more than one prompt"} {
		if !strings.Contains(h.out.String(), line) {
			t.Errorf("no %q in:\n%s", line, h.out)
		}
	}
}

func TestGcCommandForNamesTheKindsOnlyWhenTheDefaultWouldNotDo(t *testing.T) {
	items := func(whys ...string) []triageItem {
		var out []triageItem
		for _, w := range whys {
			out = append(out, triageItem{Why: w})
		}
		return out
	}
	for _, tc := range []struct {
		whys []string
		want string
	}{
		{[]string{"empty", "copy"}, "claudes session gc"},
		{[]string{"trivial"}, "claudes session gc --trivial"},
		{[]string{"leftover", "trivial", "empty"}, "claudes session gc --empty --trivial --leftovers"},
	} {
		if got := gcCommandFor(items(tc.whys...)); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.whys, got, tc.want)
		}
	}
}

// session gc ID removes the whole session, so an older copy or a leftover,
// whose session id is the original's, offers no id to pass it.
func TestOlderCopiesAndLeftoversHaveNoID(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	week := 7 * 24 * time.Hour
	moved := h.transcript(t, "moved", other, week)
	data, err := os.ReadFile(filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), moved+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	oldCopy := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), moved+".jsonl")
	writeFile(t, oldCopy, string(data))
	h.aged(t, oldCopy, 2*week)
	orphan := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), sessionID("orphan"))
	writeFile(t, filepath.Join(orphan, "subagents", "a.jsonl"), "{}")
	h.aged(t, orphan, week)
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "triage", "-o", "json")
	var triage triageReport
	if err := json.Unmarshal(h.out.Bytes(), &triage); err != nil {
		t.Fatal(err)
	}
	whys := map[string]bool{}
	for _, it := range triage.Remove {
		whys[it.Why] = true
		if it.ID != "" || it.SessionID == "" {
			t.Errorf("triage %s row: id %q, sessionId %q", it.Why, it.ID, it.SessionID)
		}
	}
	if !whys["copy"] || !whys["leftover"] {
		t.Fatalf("remove %+v", triage.Remove)
	}

	h.out.Reset()
	h.ok(t, "session", "gc", "--dry-run", "-o", "json")
	var rep report
	if err := json.Unmarshal(h.out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Plan) != 2 {
		t.Fatalf("plan %+v", rep.Plan)
	}
	for _, e := range rep.Plan {
		if e.ID != "" {
			t.Errorf("session gc %s row has id %q", e.Result, e.ID)
		}
	}

	h.out.Reset()
	h.ok(t, "session", "triage")
	if strings.Contains(h.out.String(), claude.ShortID(moved)) {
		t.Fatalf("the table shows the original's id for its copy:\n%s", h.out)
	}
}
