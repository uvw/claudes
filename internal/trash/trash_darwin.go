package trash

import (
	"os"
	"path/filepath"
)

// SystemRestore is how the system puts back what claudes moved to the trash,
// or "" when only Manifest can.
const SystemRestore = ""

// Move moves path to the Trash in the home directory, and returns where it went.
// Finder lists it there, but cannot put it back, since only Finder records where
// a file came from.
func Move(path string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return moveInto(path, filepath.Join(home, ".Trash"))
}

// forget does nothing: the Trash keeps no record of what claudes put there.
func forget(string) {}
