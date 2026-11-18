package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/transcripts"
)

func TestSessionLsShowsABranchOnlyWithWideAndFitsIt(t *testing.T) {
	h, deep, _ := deepHarness(t)
	branch, name := "feat/ledger-rounding-for-every-currency", "reconcile the ledger totals for each month"[:maxName]
	h.said(t, name, deep, time.Hour, fmt.Sprintf(`{"type":"user","gitBranch":%q,"message":{"content":"go"}}`, branch))
	cells := func(out string) (string, string) {
		for n, b := range column(t, out, "BRANCH") {
			if strings.HasPrefix(n, "reconcile") {
				return b, n
			}
		}
		t.Fatalf("no row for %q:\n%s", name, out)
		return "", ""
	}

	out := runTable(t, h, deep, []string{"session", "ls"})
	if strings.Contains(out, branchHeader) || strings.Contains(out, "ledger-rounding") {
		t.Errorf("without --wide: a BRANCH column:\n%s", out)
	}
	for _, format := range []string{"json", "yaml"} {
		if out := runTable(t, h, deep, []string{"session", "ls", "-o", format}); !strings.Contains(out, branch) {
			t.Errorf("-o %s: no whole branch:\n%s", format, out)
		}
	}
	out = runTable(t, h, deep, []string{"session", "ls", "--wide"})
	if b, n := cells(out); b != style.Branch(branch, maxBranch) || n != name {
		t.Errorf("off a terminal: BRANCH %q and NAME %q, want %q and the whole name:\n%s", b, n, style.Branch(branch, maxBranch), out)
	}
	for _, width := range []int{90, 100, 120} {
		h.app.Width = func() int { return width }
		out := runTable(t, h, deep, []string{"session", "ls", "--wide"})
		for line := range strings.Lines(out) {
			if w := style.Width(strings.TrimRight(line, "\n")); w > width {
				t.Errorf("width %d: a line of %d columns:\n%s", width, w, out)
			}
		}
		// cut drops the space before its …, so a name can end a column short.
		if b, n := cells(out); style.Width(b) < minBranch || style.Width(n) < minName-1 || style.Width(n) >= maxName {
			t.Errorf("width %d: BRANCH %q, NAME %q:\n%s", width, b, n, out)
		}
	}
}

func TestSessionLsListsTranscriptsNewestFirst(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	old := h.transcript(t, "old-plan", work, 72*time.Hour)
	live := h.bg("live", "busy")
	h.transcriptAs(t, live.SessionID, "", other, time.Minute)
	h.agents.snapshots = [][]claude.Session{{live}}

	h.ok(t, "session", "ls")
	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[1], "live") || !strings.Contains(lines[1], " yes ") ||
		!strings.HasSuffix(lines[2], "old-plan") || !strings.Contains(lines[2], "3d") {
		t.Fatalf("table:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "--since", "1d", "-o", "json")
	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["sessionId"] != live.SessionID || rows[0]["running"] != true || rows[0]["id"] != live.SessionID[:8] {
		t.Fatalf("rows %v", rows)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "--in", work)
	if !strings.Contains(h.out.String(), "old-plan") || strings.Contains(h.out.String(), "live") {
		t.Fatalf("--in:\n%s", h.out)
	}
	h.out.Reset()
	h.ok(t, "session", "ls", old[:8])
	if !strings.Contains(h.out.String(), "old-plan") || strings.Contains(h.out.String(), "live") {
		t.Fatalf("by id:\n%s", h.out)
	}
}

func TestSessionLsFiltersByRunningState(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	h.transcript(t, "old-plan", work, 72*time.Hour)
	h.transcript(t, "fresh-fix", work, 2*time.Hour)
	live := h.bg("live", "busy")
	h.transcriptAs(t, live.SessionID, "", work, time.Minute)
	h.agents.snapshots = [][]claude.Session{{live}}
	names := func(args ...string) []string {
		t.Helper()
		h.out.Reset()
		h.ok(t, append([]string{"session", "ls", "-o", "json"}, args...)...)
		var rows []pastSession
		if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.Label())
		}
		return out
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "live fresh-fix old-plan"},
		{[]string{"--running"}, "live"},
		{[]string{"--stopped"}, "fresh-fix old-plan"},
		{[]string{"--stopped", "-l", "1d"}, "fresh-fix"},
		{[]string{"--stopped", "--since", "today", "--sort", "size"}, "fresh-fix"},
		{[]string{"--running", "-l", "today", "-i", work}, "live"},
		{[]string{"--running", "--in", h.folder("other")}, ""},
	} {
		if got := strings.Join(names(c.args...), " "); got != c.want {
			t.Errorf("%q: %q, want %q", c.args, got, c.want)
		}
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "--stopped", "--wide")
	if out := h.out.String(); !strings.Contains(out, "BRANCH") || !strings.Contains(out, "old-plan") || strings.Contains(out, " yes ") {
		t.Errorf("--stopped --wide:\n%s", out)
	}

	h.err.Reset()
	h.fails(t, "session", "ls", "--running", "--stopped")
	assertContains(t, h.err.String(), "[running stopped] were all set")
}

func TestSessionLsShowsSizesAndSortsBySize(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	h.transcript(t, "small", work, time.Hour)
	big := h.transcript(t, "big", work, 48*time.Hour)
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), big, "tool-results", "r.txt"), strings.Repeat("x", 4096))
	writeFile(t, filepath.Join(h.root, "file-history", big, "f@v1"), strings.Repeat("y", 1024))
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "ls")
	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if !strings.Contains(lines[0], "SIZE") || !strings.HasSuffix(lines[1], "small") {
		t.Fatalf("table:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "--sort", "size")
	lines = strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if !strings.HasSuffix(lines[1], "big") || !strings.Contains(lines[1], "5.") {
		t.Fatalf("--sort size:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "session", "ls", "big", "-o", "json")
	var rows []pastSession
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if d := rows[0].Disk; len(rows) != 1 || d.Sidecar != 4096 || d.FileHistory != 1024 || d.Total != d.Transcript+5120 {
		t.Fatalf("rows %+v", rows)
	}

	h.fails(t, "session", "ls", "--sort", "name")
	if !strings.Contains(h.err.String(), `for "--sort" flag: must be active or size, not "name"`) {
		t.Fatalf("stderr:\n%s", h.err)
	}
}
