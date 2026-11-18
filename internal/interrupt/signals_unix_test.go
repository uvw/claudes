//go:build unix

package interrupt

import (
	"context"
	"syscall"
	"testing"
	"time"
)

// A Bash tool's timeout sends SIGTERM, and a closed terminal SIGHUP. Either
// cancels the run's context as Ctrl-C does.
func TestSIGTERMAndSIGHUPCancelARunAsCtrlCDoes(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		ctx, stop := Context(context.Background())
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
