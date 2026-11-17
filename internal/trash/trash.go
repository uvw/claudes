// Package trash moves files and directories to the system trash: the macOS Trash,
// the freedesktop.org trash on Linux and other Unix systems, and the Windows
// Recycle Bin, so a removal can be undone.
package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// moveInto renames path into dir under its own name, or under the name with a
// number added when dir already holds one. It returns the new path. A path on
// another disk than dir cannot be renamed there, and is an error.
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
		if _, err := os.Lstat(dest); err == nil {
			continue
		}
		err := os.Rename(path, dest)
		if errors.Is(err, syscall.EXDEV) {
			return "", fmt.Errorf("%s is on another disk than the trash, so it cannot go there: %w", path, err)
		}
		return dest, err
	}
}
