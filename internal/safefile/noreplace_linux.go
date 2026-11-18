//go:build linux

package safefile

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplace uses renameat2 with RENAME_NOREPLACE. Kernels before 3.15
// lack it, and some file systems refuse the flag.
func renameNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return renameChecked(from, to)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
