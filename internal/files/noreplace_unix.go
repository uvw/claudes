//go:build unix

package files

import (
	"io/fs"
	"os"
)

// renameChecked renames from to to unless to exists. Another process can
// still create to between the check and the rename, so it is the fallback
// where the system cannot refuse to replace.
func renameChecked(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: fs.ErrExist}
	}
	return os.Rename(from, to)
}
