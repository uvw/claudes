//go:build darwin

package safefile

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplace uses renamex_np with RENAME_EXCL, which some file systems,
// such as network ones, do not support.
func renameNoReplace(from, to string) error {
	err := unix.RenamexNp(from, to, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) {
		return renameChecked(from, to)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
