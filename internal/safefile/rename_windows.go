//go:build windows

package safefile

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// rename retries while another process has the target open: Windows refuses to
// replace an open file, even one opened with delete sharing, and readers such
// as agent view and antivirus scanners hold it only for a moment.
func rename(from, to string) error {
	delay := 5 * time.Millisecond
	var err error
	for range renameAttempts {
		err = os.Rename(from, to)
		if err == nil || !(errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)) {
			return err
		}
		time.Sleep(delay)
		delay = min(delay*2, 200*time.Millisecond)
	}
	return err
}

// renameAttempts with the backoff above waits a little over a second in all.
const renameAttempts = 10
