package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/uvw/claudes/internal/files"
)

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
	release, err := files.RunLock{Path: app.LockPath, Started: app.Started, Alive: app.Alive}.Acquire()
	if errors.Is(err, files.ErrHeld) {
		return fmt.Errorf("%w, so this one changed nothing. Try again once it ends; "+
			"a claudes that crashed frees %s within %s", errBusy, app.LockPath, files.RunLockStale)
	}
	if err != nil {
		return fmt.Errorf("cannot take the run lock %s: %w", app.LockPath, err)
	}
	app.unlockRun = release
	app.log().Debug("took the run lock", "path", app.LockPath)
	return nil
}

// unlock releases the run lock, if this run holds it.
func (app *App) unlock() {
	if app.unlockRun != nil {
		app.unlockRun()
		app.unlockRun = nil
		app.log().Debug("released the run lock")
	}
}
