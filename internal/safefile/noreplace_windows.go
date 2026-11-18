//go:build windows

package safefile

import (
	"os"

	"golang.org/x/sys/windows"
)

// renameNoReplace uses MoveFileEx without MOVEFILE_REPLACE_EXISTING, which
// fails when to exists.
func renameNoReplace(from, to string) error {
	f, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(f, t, 0); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
