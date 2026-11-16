//go:build unix && !darwin

package trash

import (
	"os"
	"path/filepath"
	"time"
)

// Move moves path to the home trash of the freedesktop.org Trash
// specification, which file managers on Linux list and can restore from.
func Move(path string) error {
	dir, err := freedesktopDir()
	if err != nil {
		return err
	}
	return toFreedesktop(path, dir, time.Now())
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
