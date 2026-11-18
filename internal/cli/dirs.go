package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// relWithin is path relative to root, and whether path is root or below it.
func relWithin(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// within reports whether dir is root or a directory below it.
func within(dir, root string) bool {
	if dir == "" {
		return false
	}
	_, ok := relWithin(root, dir)
	return ok
}

// resolved is dir with links followed, or dir when it is gone.
func resolved(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// sameDir reports whether a and b name the same directory, which on Windows
// can differ in case.
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}

// errNoDir is a directory that does not exist.
var errNoDir = errors.New("does not exist")

// realDir is dir as an absolute path with links resolved, as the working
// directory Claude Code records for a session started there, or the current
// directory when dir is "".
func realDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s %w", abs, errNoDir)
	}
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return resolved, nil
}

// dirGone reports whether dir is known and no longer exists. A directory that
// cannot be checked, such as one behind a permission error, does not count.
func dirGone(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(dir)
	return errors.Is(err, fs.ErrNotExist)
}

// remover is how a command removes a file or directory: it moves it to the
// system trash, or with --purge deletes it for good.
func (app *App) remover(purge bool) func(string) error {
	if purge {
		return os.RemoveAll
	}
	return app.Trash
}
