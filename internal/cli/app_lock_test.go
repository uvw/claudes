package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/safefile"
)

func TestOneClaudesChangesSessionsAtATime(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	release, err := safefile.Lock{Path: h.app.LockPath, Stale: time.Minute}.Acquire()
	if err != nil {
		t.Fatal(err)
	}

	h.exits(t, exitBusy, "stop", "api")
	assertContains(t, h.err.String(), "another claudes is changing sessions")
	h.ok(t, "restart", "--dry-run", "api")
	h.ok(t, "ls")
	if len(h.agents.calls) != 0 {
		t.Fatalf("calls %q while another claudes held the lock", h.agents.calls)
	}

	release()
	h.ok(t, "stop", "api")
	if _, err := os.Stat(h.app.LockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lock outlived the run: %v", err)
	}
}

func TestTheLockIsFreeBeforeTheHostStops(t *testing.T) {
	h := newHarness(t)
	host := h.bg("host", "idle")
	h.agents.snapshots = [][]claude.Session{{host}}
	h.host(host)
	var heldAtStop bool
	h.agents.beforeStop = func(string) {
		_, err := os.Stat(h.app.LockPath)
		heldAtStop = err == nil
	}

	h.ok(t, "stop", "host")
	if heldAtStop {
		t.Fatal("the lock was still held when the host stopped, so it would outlive claudes")
	}
}

type panicky struct{ *fakeAgents }

func (p panicky) Respawn(context.Context, string) error { panic("boom") }

func TestAPanicFreesTheRunLock(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}
	h.app.Agents = panicky{h.agents}
	func() {
		defer func() { _ = recover() }()
		h.run("restart", "api")
	}()
	if _, err := os.Stat(h.app.LockPath); err == nil {
		t.Fatal("the run lock outlived a panic")
	}
}

// A claudes suspended mid-run, such as by a closed laptop lid, stops
// refreshing the run lock. The lock names its process, so it stays while that
// process runs, and goes once it has ended.
func TestTheRunLockOutlivesASuspendButNotItsOwner(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	if err := os.MkdirAll(h.app.LockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.app.LockPath, "owner"), fmt.Sprintf("4242 %s\n", start.Format(time.RFC3339Nano)))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(h.app.LockPath, old, old); err != nil {
		t.Fatal(err)
	}
	h.app.Alive = func(pid int, started time.Time) bool { return pid == 4242 && started.Equal(start) }

	h.exits(t, exitBusy, "stop", "api")
	assertCalls(t, h)

	h.app.Alive = func(int, time.Time) bool { return false }
	h.ok(t, "stop", "api")
	assertCalls(t, h, "stop "+s.JobID)
}

func TestTheRunLockNamesItsProcess(t *testing.T) {
	h := newHarness(t)
	owner := h.app.lockOwner()
	h.app.Alive = func(pid int, started time.Time) bool { return pid == os.Getpid() && started.Equal(start) }
	if h.app.lockOwnerGone(owner) {
		t.Fatalf("owner %q of this process reads as gone", owner)
	}
	for _, bad := range []string{"", "x", "0 " + start.Format(time.RFC3339Nano), "42 yesterday"} {
		if !h.app.lockOwnerGone([]byte(bad)) {
			t.Errorf("owner %q reads as running", bad)
		}
	}
}
