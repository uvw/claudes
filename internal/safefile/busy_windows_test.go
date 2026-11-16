//go:build windows

package safefile

import (
	"fmt"
	"io/fs"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestAccessDeniedWhileALockIsRemovedCountsAsHeld(t *testing.T) {
	denied := &os.PathError{Op: "mkdir", Path: "x.lock", Err: windows.ERROR_ACCESS_DENIED}
	for err, want := range map[error]bool{
		denied:                            true,
		fmt.Errorf("wrapped: %w", denied): true,
		&os.PathError{Op: "mkdir", Path: "x.lock", Err: fs.ErrExist}:                  true,
		&os.PathError{Op: "mkdir", Path: "x.lock", Err: windows.ERROR_PATH_NOT_FOUND}: false,
	} {
		if got := busy(err); got != want {
			t.Errorf("busy(%v) = %v, want %v", err, got, want)
		}
	}
}
