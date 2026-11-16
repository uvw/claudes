package trash

import (
	"os"
	"path/filepath"
)

// Move moves path to the Trash in the home folder. Finder lists it there, but
// cannot put it back, since only Finder records where a file came from.
func Move(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	_, err = moveInto(path, filepath.Join(home, ".Trash"))
	return err
}
