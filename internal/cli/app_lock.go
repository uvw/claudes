package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/files"
)

// lockStale is how long the run lock outlives a claudes that died holding it.
const lockStale = 30 * time.Second

// runsAlone marks a command that changes sessions or pins. One such command
// runs at a time, so two cannot stop the same session twice.
const runsAlone = "runs-alone"

func alone(cmd *cobra.Command) *cobra.Command {
	cmd.Annotations = map[string]string{runsAlone: "true"}
	return cmd
}

// lock takes the run lock for a command marked runsAlone, unless it is a dry
// run, which changes nothing.
func (app *App) lock(cmd *cobra.Command) error {
	if cmd.Annotations[runsAlone] == "" || app.LockPath == "" || app.unlockRun != nil {
		return nil
	}
	if f := cmd.Flags().Lookup("dry-run"); f != nil && f.Value.String() == "true" {
		return nil
	}
	release, err := files.Lock{Path: app.LockPath, Stale: lockStale, Attempts: 1, Owner: app.lockOwner(), Gone: app.lockOwnerGone}.Acquire()
	if errors.Is(err, files.ErrHeld) {
		return fmt.Errorf("%w, so this one changed nothing. Try again once it ends; "+
			"a claudes that crashed frees %s within %s", errBusy, app.LockPath, lockStale)
	}
	if err != nil {
		return fmt.Errorf("cannot take the run lock %s: %w", app.LockPath, err)
	}
	app.unlockRun = release
	app.log().Debug("took the run lock", "path", app.LockPath)
	return nil
}

// lockOwner is what the run lock's owner file holds: this process's PID and
// when it started, so a run that wakes from a suspend keeps its lock.
func (app *App) lockOwner() []byte {
	pid := os.Getpid()
	var started time.Time
	if app.Started != nil {
		started, _ = app.Started(pid)
	}
	return fmt.Appendf(nil, "%d %s\n", pid, started.Format(time.RFC3339Nano))
}

// lockOwnerGone reports whether the claudes a run lock's owner file names has
// ended. An owner file claudes cannot read names no one.
func (app *App) lockOwnerGone(owner []byte) bool {
	var pid int
	var at string
	if _, err := fmt.Sscan(string(owner), &pid, &at); err != nil || pid <= 0 || app.Alive == nil {
		return true
	}
	started, err := time.Parse(time.RFC3339Nano, at)
	return err != nil || !app.Alive(pid, started)
}

// unlock releases the run lock, if this run holds it.
func (app *App) unlock() {
	if app.unlockRun != nil {
		app.unlockRun()
		app.unlockRun = nil
		app.log().Debug("released the run lock")
	}
}
