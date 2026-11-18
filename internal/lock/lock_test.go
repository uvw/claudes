package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var start = time.Date(2026, 11, 13, 17, 0, 0, 0, time.UTC)

func started(int) (time.Time, bool) { return start, true }

func TestTheRunLockNamesItsProcess(t *testing.T) {
	r := Run{
		Started: started,
		Alive:   func(pid int, at time.Time) bool { return pid == os.Getpid() && at.Equal(start) },
	}
	owner := r.owner()
	if r.ownerGone(owner) {
		t.Fatalf("owner %q of this process reads as gone", owner)
	}
	for _, bad := range []string{"", "x", "0 " + start.Format(time.RFC3339Nano), "42 yesterday"} {
		if !r.ownerGone([]byte(bad)) {
			t.Errorf("owner %q reads as running", bad)
		}
	}
	if !(Run{}).ownerGone(owner) {
		t.Error("an owner reads as running with no way to tell")
	}
}

// A claudes suspended mid-run stops refreshing the run lock. The lock stays
// while its owner runs, and goes once the owner has ended.
func TestAStaleLockStaysWhileItsOwnerRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("4242 %s\n", start.Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(path, "owner"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	alive := true
	r := Run{Path: path, Started: started, Alive: func(pid int, at time.Time) bool {
		return alive && pid == 4242 && at.Equal(start)
	}}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Acquire(); !errors.Is(err, ErrHeld) {
		t.Fatalf("took a lock whose owner runs: %v", err)
	}
	alive = false
	release, err := r.Acquire()
	if err != nil {
		t.Fatalf("kept a lock whose owner ended: %v", err)
	}
	release()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("release left the lock: %v", err)
	}
}
