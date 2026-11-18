package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
	"github.com/uvw/claudes/internal/transcripts"
)

// A record that yields two events is one transcript record; -o json promises
// "the transcript records themselves, one per line".
func TestLogsJSONPrintsEachRecordOnce(t *testing.T) {
	h := newHarness(t)
	s := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s,
		`{"type":"user","message":{"content":"<teammate-message teammate_id=\"a\">hi</teammate-message><teammate-message teammate_id=\"b\">yo</teammate-message>"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
	)
	h.ok(t, "logs", "lead", "--tail", "10", "-o", "json")
	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("2 records printed as %d lines:\n%s", len(lines), h.out)
	}
}

// --tail N counts events, so a record that yields several events can push
// --tail 1 -o json to print the same record, and never the one before it.
func TestLogsTailCountsRecordsInJSON(t *testing.T) {
	h := newHarness(t)
	s := h.bg("lead", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s,
		`{"type":"user","message":{"content":"first"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
	)
	h.ok(t, "logs", "lead", "--tail", "2", "-o", "json")
	if !strings.Contains(h.out.String(), "first") {
		t.Fatalf("--tail 2 -o json left out the second-to-last record:\n%s", h.out)
	}
}

// Named subagents of a session that leads no team are not taken for teammates,
// and a teammate the team lacks is a target that names nothing, exit 2.
func TestLogsLeadSlashNameNeedsATeam(t *testing.T) {
	h := newHarness(t)
	solo := h.bg("solo", "busy")
	h.agents.snapshots = [][]claude.Session{{solo}}
	path := h.logLines(t, solo, `{"type":"user","message":{"content":"go"}}`)
	writeFile(t, filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-adeps-d27ce76e9061ede7.jsonl"),
		`{"type":"assistant","message":{"content":[{"type":"text","text":"a subagent"}]}}`+"\n")
	h.exits(t, exit.NoMatch, "logs", "solo/deps")
	h.exits(t, exit.NoMatch, "logs", "solo/nobody")
}

// Session names often hold a slash, like a branch name.
func TestLogsOfASessionNamedWithASlash(t *testing.T) {
	h := newHarness(t)
	s := h.bg("fix/auth", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s, `{"type":"user","message":{"content":"go"}}`)
	h.ok(t, "where", "fix/auth")
	if code := h.run("logs", "fix/auth"); code != 0 {
		t.Errorf("logs fix/auth exits %d: %s", code, h.err)
	}
	h.err.Reset()
	if code := h.run("logs", "fix/auth", "--tail", "5"); code != 0 {
		t.Errorf("logs fix/auth --tail 5 exits %d: %s", code, h.err)
	}
}

func followWith(t *testing.T, h *harness, steps ...func(path string)) string {
	t.Helper()
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	path := h.logLines(t, s, `{"type":"user","message":{"content":"old"}}`)
	n := 0
	h.app.Sleep = func(ctx context.Context, _ time.Duration) error {
		if n == len(steps) {
			h.interrupt()
			return ctx.Err()
		}
		steps[n](path)
		n++
		return nil
	}
	h.ok(t, "logs", "-f", "api", "--tail", "0")
	return h.out.String()
}

func appendTo(t *testing.T, path, text string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(f, text)
	f.Close()
}

func TestFollowPrintsALineSplitAcrossPollsOnce(t *testing.T) {
	h := newHarness(t)
	out := followWith(t, h,
		func(p string) { appendTo(t, p, `{"type":"user","message":{"cont`) },
		func(p string) { appendTo(t, p, `ent":"split"}}`+"\n") },
		func(p string) {},
	)
	if strings.Count(out, "split") != 1 {
		t.Fatalf("output:\n%s", out)
	}
}

// A transcript replaced by a longer one is read from the old offset, mid-line.
func TestFollowNoticesAReplacedTranscript(t *testing.T) {
	h := newHarness(t)
	out := followWith(t, h,
		func(p string) {
			os.WriteFile(p, []byte(`{"type":"user","message":{"content":"replaced one"}}`+"\n"+`{"type":"user","message":{"content":"replaced two"}}`+"\n"), 0o644)
		},
		func(p string) {},
	)
	if !strings.Contains(out, "replaced one") {
		t.Fatalf("skipped the first line of the new transcript:\n%s\n%s", out, h.err)
	}
}

func TestLogsAttachAndWhere(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}

	h.ok(t, "logs", "alp")
	h.ok(t, "attach", "alpha")
	h.ok(t, "where", "alpha")

	want := [][]string{{"claude", "logs", a.JobID}, {"claude", "attach", a.JobID}}
	if len(h.execs) != 2 || !slices.Equal(h.execs[0], want[0]) || !slices.Equal(h.execs[1], want[1]) {
		t.Fatalf("execs %q", h.execs)
	}
	assertContains(t, h.out.String(), a.Cwd)
}

func (h *harness) logLines(t *testing.T, s claude.Session, lines ...string) string {
	t.Helper()
	path := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(s.Cwd), s.SessionID+".jsonl")
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	return path
}

func TestLogsTailPrintsTheLastEvents(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.logLines(t, s,
		`{"type":"user","message":{"content":"one"}}`,
		`{"type":"user","message":{"content":"two"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"three"}]}}`,
	)

	h.ok(t, "logs", "api", "--tail", "2")
	out := h.out.String()
	if strings.Contains(out, "one") || !strings.Contains(out, "you    two") || !strings.Contains(out, "claude three") {
		t.Fatalf("output:\n%s", out)
	}
	if len(h.execs) != 0 {
		t.Fatalf("handed off to claude logs: %q", h.execs)
	}

	h.out.Reset()
	h.ok(t, "logs", "api", "--tail", "1", "-o", "json")
	if got := strings.TrimSpace(h.out.String()); got != `{"type":"assistant","message":{"content":[{"type":"text","text":"three"}]}}` {
		t.Fatalf("json %q", got)
	}
}

func TestLogsFollowPrintsNewEventsUntilCtrlC(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	path := h.logLines(t, s, `{"type":"user","message":{"content":"old"}}`)
	sleeps := 0
	h.app.Sleep = func(ctx context.Context, _ time.Duration) error {
		sleeps++
		switch sleeps {
		case 1:
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprint(f, `{"type":"assistant","message":{"content":[{"type":"text","text":"new"}]}}`+"\n"+`{"type":"user","message":{"cont`)
			f.Close()
		case 2:
			h.interrupt()
			return ctx.Err()
		}
		return nil
	}

	h.ok(t, "logs", "-f", "api", "--tail", "0")
	out := h.out.String()
	if strings.Contains(out, "old") || !strings.Contains(out, "claude new") {
		t.Fatalf("output:\n%s", out)
	}
	if strings.Contains(h.err.String(), "interrupted") {
		t.Fatalf("Ctrl-C was reported as an interruption:\n%s", h.err)
	}
}

func TestLogsWithoutFlagsHandsOffToClaude(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}

	h.ok(t, "logs", "api")
	if len(h.execs) != 1 || !slices.Equal(h.execs[0], []string{"claude", "logs", s.JobID}) {
		t.Fatalf("execs %q", h.execs)
	}
}

func TestLogsShowsATeammateByLeadAndName(t *testing.T) {
	h := newHarness(t)
	lead := h.bg("lead", "busy")
	h.agents.snapshots = [][]claude.Session{{lead}}
	path := h.logLines(t, lead, `{"type":"user","message":{"content":"go"}}`)
	writeFile(t, filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-atester-abcd.jsonl"),
		`{"type":"assistant","message":{"content":[{"type":"text","text":"tests pass"}]}}`+"\n")
	writeFile(t, filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-atester-abcd.meta.json"), `{"teamName":"session-x","taskKind":"in_process_teammate"}`)

	h.ok(t, "logs", "lead/tester")
	assertContains(t, h.out.String(), "claude tests pass")
	if len(h.execs) != 0 {
		t.Fatalf("handed off to claude logs: %q", h.execs)
	}
	h.exits(t, 2, "logs", "lead/nobody")
	assertContains(t, h.err.String(), "teammates: tester")
}
