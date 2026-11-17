//go:build windows

package safefile

import (
	"errors"
	"io/fs"

	"golang.org/x/sys/windows"
)

// busy reports whether a failed Mkdir of a lock means another process holds
// it, rather than that the directory cannot be made at all. While another process
// removes the lock directory, Windows answers a new Mkdir with access denied
// instead of "already exists", so that counts as held too.
func busy(err error) bool {
	return errors.Is(err, fs.ErrExist) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
