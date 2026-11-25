package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestSessionResumeRunsClaudeInTheSessionsFolder(t *testing.T) {
	h := newHarness(t)
	dir := h.folder("work")
	id := h.transcript(t, "plan", dir, time.Hour)

	h.ok(t, "session", "resume", "plan", "keep going")
	if !slices.Equal(h.chdirs, []string{dir}) || len(h.execs) != 1 ||
		!slices.Equal(h.execs[0], []string{"claude", "--resume", id, "keep going"}) {
		t.Fatalf("chdirs %q, execs %q", h.chdirs, h.execs)
	}

	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+id+" in work prompt= name=plan")
}

func TestSessionResumeRefusesRunningMissingAndOptionLikeInput(t *testing.T) {
	h := newHarness(t)
	live := h.bg("live", "idle")
	h.transcriptAs(t, live.SessionID, "live", h.folder("live"), time.Minute)
	h.agents.snapshots = [][]claude.Session{{live}}
	h.transcript(t, "gone", filepath.Join(h.root, "deleted"), time.Hour)

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"live"}, "would start a copy"},
		{[]string{"gone"}, "which is gone"},
		{[]string{"gone", "--", "--model"}, "read it as an option"},
	} {
		h.err.Reset()
		h.fails(t, append([]string{"session", "resume"}, c.args...)...)
		assertContains(t, h.err.String(), c.want)
	}
	if len(h.execs) != 0 || len(h.agents.calls) != 0 {
		t.Fatalf("execs %q, calls %q", h.execs, h.agents.calls)
	}
}

func TestSessionResumeBgRefusesAJobThatWouldWakeInTheOldFolder(t *testing.T) {
	h := newHarness(t)
	from, to := h.folder("from"), h.folder("to")
	id := h.transcript(t, "plan", from, time.Hour)
	h.ok(t, "session", "mv", "plan", to)
	job := claude.ShortID(id)
	writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, job, "state.json"), fmt.Sprintf(`{"sessionId":%q,"cwd":%q}`, id, from))

	h.fails(t, "session", "resume", "--bg", "plan")
	assertContains(t, h.err.String(), "would wake it there")
	assertCalls(t, h)
}

func TestSessionResumeBgNamesASessionThatHasNoJob(t *testing.T) {
	h := newHarness(t)
	id := h.transcript(t, "plan", h.folder("work"), time.Hour)

	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+id+" in work prompt= name=plan")

	writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, claude.ShortID(id), "state.json"), fmt.Sprintf(`{"sessionId":%q}`, id))
	h.agents.calls = nil
	h.agents.resumed = nil
	h.ok(t, "session", "resume", "--bg", "plan")
	assertCalls(t, h, "resume "+id+" in work prompt=")
}
