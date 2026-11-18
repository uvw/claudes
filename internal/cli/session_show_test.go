package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/transcripts"
)

func TestSessionShowWritesTimesInUTCAndAnEmptyCopyList(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("one"), time.Hour)

	h.ok(t, "session", "show", "plan", "-o", "json")
	var got map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, h.out)
	}
	if copies, ok := got["copies"].([]any); !ok || len(copies) != 0 {
		t.Fatalf("copies %#v, want []", got["copies"])
	}
	if s, _ := got["lastActive"].(string); !strings.HasSuffix(s, "Z") {
		t.Fatalf("lastActive %q, want UTC", s)
	}
}

// A project directory's name reaches session show's older copies as is, so
// an escape sequence in it must not reach the terminal.
func TestSessionShowPrintsOlderCopiesPrintable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file names cannot hold control characters")
	}
	h := newHarness(t)
	id := h.transcript(t, "plan", h.folder("one"), time.Hour)
	src := filepath.Join(h.app.Transcripts.Dir, transcripts.Folder(h.folder("one")), id+".jsonl")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	evil := filepath.Join(h.app.Transcripts.Dir, "-evil\x1b[2J", id+".jsonl")
	writeFile(t, evil, string(data))
	old := h.now.Add(-48 * time.Hour)
	if err := os.Chtimes(evil, old, old); err != nil {
		t.Fatal(err)
	}

	h.ok(t, "session", "show", "plan")
	assertContains(t, h.out.String(), "older copy:")
	if strings.ContainsRune(h.out.String(), 0x1b) {
		t.Fatalf("session show printed an escape sequence: %q", h.out.String())
	}
}

func TestSessionShowPrintsTheSize(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "plan", h.folder("work"), time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}

	h.ok(t, "session", "show", "plan")
	if !strings.Contains(h.out.String(), "size:") || !strings.Contains(h.out.String(), "file history 0B") {
		t.Fatalf("show:\n%s", h.out)
	}
}
