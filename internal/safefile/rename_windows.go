//go:build windows

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// rename replaces to with from, a file in the same directory. os.Root.Rename
// tries POSIX semantics first, which replace a file that other processes have
// open if each opened it with delete sharing, as Node and Bun do, and falls
// back to a plain rename, which cannot replace an open file at all. Either way
// a held file gets retries: Go's os.Open does not share delete, and antivirus
// scanners hold a file only for a moment.
func rename(from, to string) error {
	dir := filepath.Dir(to)
	if filepath.Dir(from) != dir {
		return retry(func() error { return os.Rename(from, to) })
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return retry(func() error { return root.Rename(filepath.Base(from), filepath.Base(to)) })
}

// retry calls rename until it succeeds, fails for a reason other than an open
// target, or runs out of attempts.
func retry(rename func() error) error {
	delay := 5 * time.Millisecond
	for try := 1; ; try++ {
		err := rename()
		if err == nil || !isOpen(err) || try == renameAttempts {
			return err
		}
		sleep(delay)
		delay = min(delay*2, 200*time.Millisecond)
	}
}

// isOpen reports whether a rename failed because another process has the
// target open.
func isOpen(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

// renameAttempts with the backoff above waits a little under a second in all.
const renameAttempts = 10

// sleep is time.Sleep, which tests replace to count the waits.
var sleep = time.Sleep
