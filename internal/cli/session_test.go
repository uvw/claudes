package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestSessionAnswersToSAndSessions(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "old-plan", h.folder("work"), 72*time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}

	for _, alias := range []string{"s", "sessions"} {
		h.out.Reset()
		h.ok(t, alias, "ls")
		if !strings.Contains(h.out.String(), "old-plan") {
			t.Fatalf("%s ls:\n%s", alias, h.out)
		}
	}
	if len(h.execs) != 0 {
		t.Fatalf("an alias passed through to claude agents: %q", h.execs)
	}
}

func TestSessionCommandsRefuseAnEmptyTarget(t *testing.T) {
	h := newHarness(t)
	h.transcript(t, "", h.folder("from"), time.Hour)
	h.fails(t, "session", "show", "")
	assertContains(t, h.err.String(), "empty target")
}

// A transcript can record its directory through a link, as macOS does for the
// temporary directory under /var.
func TestSessionCommandsSeeAFolderRecordedThroughALink(t *testing.T) {
	h := newHarness(t)
	real := h.folder("real")
	link := filepath.Join(h.root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot make a link: %v", err)
	}
	h.transcript(t, "plan", link, time.Hour)

	h.ok(t, "session", "ls", "--in", real)
	assertContains(t, h.out.String(), "plan")

	h.fails(t, "session", "mv", "plan", real)
	assertContains(t, h.err.String(), "already lives under")
}
