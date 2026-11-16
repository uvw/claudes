//go:build windows

package selfupdate

import "os"

// swap puts staged in place of exe. Windows cannot overwrite a running
// program, but it can rename one, so exe moves aside to exe.old first. The
// .old file is removed the next time swap runs, once nothing runs it.
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
	return nil
}
