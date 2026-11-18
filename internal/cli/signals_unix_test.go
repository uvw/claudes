//go:build unix

package cli

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
)

// A Bash tool's timeout sends SIGTERM, and a closed terminal SIGHUP. Either
// cancels the run's context as Ctrl-C does.
func TestSIGTERMAndSIGHUPCancelARunAsCtrlCDoes(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		ctx, stop := interruptContext(context.Background())
		if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
			t.Errorf("%v did not cancel the run", sig)
		}
		stop()
	}
}

// restart finishes the session in hand, names the rest, and exits 130 on a
// SIGTERM, as it does on Ctrl-C.
func TestASIGTERMDuringRestartReportsTheRest(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}
	h.app.Interrupt = interruptContext
	var once sync.Once
	h.agents.beforeRespawn = func(string) {
		once.Do(func() { _ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM) })
		// The signal reaches the run's context on another goroutine.
		time.Sleep(100 * time.Millisecond)
	}

	if code := h.run("restart"); code != exit.Interrupted {
		t.Fatalf("exit %d, want %d:\n%s\n%s", code, exit.Interrupted, h.out, h.err)
	}
	assertCalls(t, h, "respawn "+a.JobID)
	assertContains(t, h.out.String(), "restarted  alpha")
	assertContains(t, h.err.String(), "Not reached after Ctrl-C, left as they were: beta")
}
