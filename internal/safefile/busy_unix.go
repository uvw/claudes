//go:build unix

package safefile

import (
	"errors"
	"io/fs"
)

// busy reports whether a failed Mkdir of a lock means another process holds
// it, rather than that the folder cannot be made at all.
func busy(err error) bool { return errors.Is(err, fs.ErrExist) }
