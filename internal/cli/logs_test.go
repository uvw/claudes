package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
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
	h.exits(t, exitNoMatch, "logs", "solo/deps")
	h.exits(t, exitNoMatch, "logs", "solo/nobody")
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
