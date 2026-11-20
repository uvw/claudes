//go:build windows

package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// pair makes a target holding "old" and a file holding "new" to replace it.
func pair(t *testing.T) (from, to string) {
	dir := t.TempDir()
	from, to = filepath.Join(dir, "pins.json.123"), filepath.Join(dir, "pins.json")
	if err := os.WriteFile(to, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(from, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	return from, to
}

func wantContent(t *testing.T, path, want string) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("%s holds %q (%v), want %q", path, got, err, want)
	}
}

// openSharingDelete opens path the way Node and Bun do.
func openSharingDelete(t *testing.T, path string) *os.File {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(h), path)
	t.Cleanup(func() { f.Close() })
	return f
}

// countSleeps replaces sleep for the test, and returns the count of waits.
func countSleeps(t *testing.T) *int {
	sleeps := 0
	sleep = func(time.Duration) { sleeps++ }
	t.Cleanup(func() { sleep = time.Sleep })
	return &sleeps
}

// Agent view keeps pins.json open, with delete sharing, while claudes
// replaces it.
func TestRenameReplacesAFileOpenWithDeleteSharingAtOnce(t *testing.T) {
	from, to := pair(t)
	reader := openSharingDelete(t, to)
	sleeps := countSleeps(t)
	if err := rename(from, to); err != nil {
		t.Fatalf("rename over an open file: %v", err)
	}
	wantContent(t, to, "new")
	if _, err := os.Stat(from); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source left behind: %v", err)
	}
	if got, err := io.ReadAll(reader); err != nil || string(got) != "old" {
		t.Fatalf("the open handle reads %q (%v), want the old file", got, err)
	}
	if *sleeps != 0 {
		t.Fatalf("waited %d times for a reader that shares delete", *sleeps)
	}
}

func TestRenameWaitsForAReaderNotSharingDelete(t *testing.T) {
	from, to := pair(t)
	// Go's os.Open shares read and write, but not delete.
	reader, err := os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		reader.Close()
	}()
	if err := rename(from, to); err != nil {
		t.Fatal(err)
	}
	wantContent(t, to, "new")
}

func TestRenameTakesARelativePath(t *testing.T) {
	from, to := pair(t)
	t.Chdir(filepath.Dir(to))
	if err := rename(filepath.Base(from), filepath.Base(to)); err != nil {
		t.Fatal(err)
	}
	wantContent(t, to, "new")
}

func TestRenameGivesUpOnAFileThatStaysOpen(t *testing.T) {
	from, to := pair(t)
	reader, err := os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	sleeps := countSleeps(t)
	if err := rename(from, to); !isOpen(err) {
		t.Fatalf("err %v, want access denied or a sharing violation", err)
	}
	wantContent(t, to, "old")
	wantContent(t, from, "new")
	if *sleeps != renameAttempts-1 {
		t.Fatalf("slept %d times over %d attempts, want one fewer", *sleeps, renameAttempts)
	}
}
