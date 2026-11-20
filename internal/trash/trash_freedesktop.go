//go:build unix && !darwin

package trash

import (
	"os"
	"path/filepath"
	"time"
)

// SystemRestore is how the system puts back what claudes moved to the trash,
// or "" when only Manifest can. A file manager can, but Manifest restores a
// session's files together.
const SystemRestore = ""

// Move moves path to the home trash of the freedesktop.org Trash
// specification, which file managers on Linux list and can restore from, and
// returns where it went.
func Move(path string) (string, error) {
	dir, err := freedesktopDir()
	if err != nil {
		return "", err
	}
	return toFreedesktop(path, dir, time.Now())
}

// forget removes the .trashinfo file of trashed once it is out of the trash,
// so file managers stop listing it.
func forget(trashed string) {
	if info := trashInfo(trashed); info != "" {
		os.Remove(info)
	}
}

// freedesktopDir is the home trash of the freedesktop.org Trash
// specification: $XDG_DATA_HOME/Trash, or ~/.local/share/Trash.
func freedesktopDir() (string, error) {
	if data := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(data) {
		return filepath.Join(data, "Trash"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "Trash"), nil
}
