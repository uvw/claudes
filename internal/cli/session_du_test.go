package cli

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/transcripts"
)

func TestSessionDuAddsUpByFolderWithLeftovers(t *testing.T) {
	h := newHarness(t)
	work, other := h.folder("work"), h.folder("other")
	a := h.transcript(t, "a", work, time.Hour)
	h.transcript(t, "b", work, 2*time.Hour)
	h.transcript(t, "c", other, 3*time.Hour)
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), a, "tool-results", "r.txt"), strings.Repeat("x", 2048))
	orphan := sessionID("orphan")
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), orphan, "tool-results", "r.txt"), strings.Repeat("z", 300))
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(other), "memory", "MEMORY.md"), "notes")
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "du", "-o", "json")
	var report duReport
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Dirs) != 2 || report.Dirs[0].Dir != work || report.Dirs[0].Sessions != 2 ||
		report.Dirs[0].Disk.Sidecar != 2048 || report.Dirs[1].Sessions != 1 {
		t.Fatalf("directories %+v", report.Dirs)
	}
	if len(report.Leftovers) != 1 || report.Leftovers[0].SessionID != orphan || report.Leftovers[0].Size != 300 {
		t.Fatalf("leftovers %+v", report.Leftovers)
	}
	if want := report.Dirs[0].Disk.Total + report.Dirs[1].Disk.Total + 300; report.Total != want {
		t.Fatalf("total %d, want %d", report.Total, want)
	}

	h.out.Reset()
	h.ok(t, "session", "df")
	text := h.out.String()
	if !strings.Contains(text, "1 leftover dir(s)") || !strings.Contains(text, "all") {
		t.Fatalf("df:\n%s", text)
	}

	h.out.Reset()
	h.ok(t, "session", "du", "--in", other, "-o", "json")
	report = duReport{}
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Dirs) != 1 || len(report.Leftovers) != 0 {
		t.Fatalf("--in counted other directories or leftovers: %+v", report)
	}
}

func TestSessionDuTopListsTheBiggestSessionsWithTheirSidecarShare(t *testing.T) {
	h := newHarness(t)
	work := h.folder("work")
	h.transcript(t, "small", work, time.Hour)
	big := h.transcript(t, "big", work, 2*time.Hour)
	mid := h.transcript(t, "mid", work, 3*time.Hour)
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), big, "tool-results", "r.txt"), strings.Repeat("x", 9000))
	writeFile(t, filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(work), mid, "tool-results", "r.txt"), strings.Repeat("x", 3000))
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "du", "--top", "2", "-o", "json")
	var report duReport
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Top) != 2 || report.Top[0].SessionID != big || report.Top[1].SessionID != mid || report.Top[0].Disk.Sidecar != 9000 {
		t.Fatalf("top %+v", report.Top)
	}

	h.out.Reset()
	h.ok(t, "session", "du", "--top", "5")
	text := h.out.String()
	if !strings.Contains(text, "The 3 biggest session(s):") || strings.Index(text, "big") > strings.Index(text, "small") {
		t.Fatalf("output:\n%s", text)
	}
	if !regexp.MustCompile(`(?m)8\.8K\s+9\d%\s+\S+\s+big$`).MatchString(text) {
		t.Fatalf("no sidecar share for big:\n%s", text)
	}

	h.out.Reset()
	h.ok(t, "session", "du", "-o", "json")
	if !strings.Contains(h.out.String(), `"dirs": [`) || !strings.Contains(h.out.String(), `"dir": "`) {
		t.Fatalf("du JSON does not use dirs and dir:\n%s", h.out)
	}
	if !strings.Contains(h.out.String(), `"top": []`) {
		t.Fatalf("top is not an empty list without --top:\n%s", h.out)
	}

	h.fails(t, "session", "du", "--top", "-1")
	assertContains(t, h.err.String(), `invalid argument "-1" for "-t, --top" flag: must be a whole number, 0 or more`)
}
