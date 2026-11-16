//go:build windows

package selfupdate

import "os"

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
		os.Rename(old, exe)
		return err
	}
	os.Remove(old)
	return nil
}
