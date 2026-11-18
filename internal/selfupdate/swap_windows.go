//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

// swap puts staged in place of exe. Windows cannot overwrite a running
// program, but it can rename one, so exe moves aside to exe.old first. The
// .old file is removed at once when nothing runs it, and otherwise the next
// time swap runs.
func swap(staged, exe string) error {
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staged, exe); err != nil {
		if back := os.Rename(old, exe); back != nil {
			return fmt.Errorf("%w; the old claudes could not be put back either, and is at %s: %w", err, old, back)
		}
		return err
	}
	os.Remove(old)
	return nil
}
