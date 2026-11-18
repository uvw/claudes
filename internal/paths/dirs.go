package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RelWithin is path relative to root, and whether path is root or below it.
func RelWithin(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// Within reports whether dir is root or a directory below it.
func Within(dir, root string) bool {
	if dir == "" {
		return false
	}
	_, ok := RelWithin(root, dir)
	return ok
}

// Resolved is dir with links followed, or dir when it is gone.
func Resolved(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// Same reports whether a and b name the same directory, which on Windows can
// differ in case.
func Same(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}

// ErrNoDir is a directory that does not exist.
var ErrNoDir = errors.New("does not exist")

// Real is dir as an absolute path with links resolved, as the working
// directory Claude Code records for a session started there, or the current
// directory when dir is "".
func Real(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s %w", abs, ErrNoDir)
	}
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return resolved, nil
}

// Gone reports whether dir is known and no longer exists. A directory that
// cannot be checked, such as one behind a permission error, does not count.
func Gone(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(dir)
	return errors.Is(err, fs.ErrNotExist)
}

// IsDir reports whether dir is an existing directory.
func IsDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
