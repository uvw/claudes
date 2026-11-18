// Package lock is the run lock, which lets one claudes at a time change
// sessions. It is a files.Lock whose owner file names the claudes holding it.
package lock

import (
	"fmt"
	"os"
	"time"

	"github.com/uvw/claudes/internal/files"
)

// Stale is how long the run lock outlives a claudes that died holding it.
const Stale = 30 * time.Second

// ErrHeld means another claudes holds the run lock.
var ErrHeld = files.ErrHeld

// Run is the run lock at Path.
type Run struct {
	Path string
	// Started is when a process started, if it runs.
	Started func(pid int) (time.Time, bool)
	// Alive reports whether the process that started at started still runs.
	Alive func(pid int, started time.Time) bool
}

// Acquire takes the lock, or returns ErrHeld at once when another claudes
// holds it.
func (r Run) Acquire() (release func(), err error) {
	return files.Lock{Path: r.Path, Stale: Stale, Attempts: 1, Owner: r.owner(), Gone: r.ownerGone}.Acquire()
}

// owner is what the run lock's owner file holds: this process's PID and when
// it started, so a run that wakes from a suspend keeps its lock.
func (r Run) owner() []byte {
	pid := os.Getpid()
	var started time.Time
	if r.Started != nil {
		started, _ = r.Started(pid)
	}
	return fmt.Appendf(nil, "%d %s\n", pid, started.Format(time.RFC3339Nano))
}

// ownerGone reports whether the claudes a run lock's owner file names has
// ended. An owner file claudes cannot read names no one.
func (r Run) ownerGone(owner []byte) bool {
	var pid int
	var at string
	if _, err := fmt.Sscan(string(owner), &pid, &at); err != nil || pid <= 0 || r.Alive == nil {
		return true
	}
	started, err := time.Parse(time.RFC3339Nano, at)
	return err != nil || !r.Alive(pid, started)
}
