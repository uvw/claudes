//go:build unix

package procs

import (
	"testing"
	"time"
)

func TestStopAllSignalsOnlyAProcessWithItsListedStart(t *testing.T) {
	h := start(t, "claude", "sleep")
	started, ok := Started(h.pid)
	if !ok {
		t.Fatal("the process is not running")
	}
	if err := stopAll(h.pid, map[int]time.Time{h.pid: started.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if !Alive(h.pid) {
		t.Fatal("a process whose start differs from the listed one, as a reused PID's does, was signalled")
	}
	if err := stopAll(h.pid, map[int]time.Time{h.pid: started}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the process with its listed start is still running")
	}
}
