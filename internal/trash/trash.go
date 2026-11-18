// Package trash moves files and directories to the system trash: the macOS Trash,
// the freedesktop.org trash on Linux and other Unix systems, and the Windows
// Recycle Bin, so a removal can be undone. Windows deletes an item too big for
// the Recycle Bin, or on a drive without one, for good.
package trash

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/uvw/claudes/internal/files"
)

// moveInto renames path into dir under its own name, or under the name with a
// number added when dir already holds one, even one that appears during the
// rename. It returns the new path. A path on another disk than dir cannot be
// renamed there, and is an error.
func moveInto(path, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := filepath.Base(path)
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s %d", base, i)
		}
		dest := filepath.Join(dir, name)
		err := files.RenameNoReplace(path, dest)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if errors.Is(err, syscall.EXDEV) {
			return "", fmt.Errorf("%s is on another disk than the trash, so it cannot go there: %w", path, err)
		}
		return dest, err
	}
}
