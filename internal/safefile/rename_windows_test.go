//go:build windows

package safefile

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

func TestRenameReplacesAFileOpenWithDeleteSharing(t *testing.T) {
	from, to := pair(t)
	reader := openSharingDelete(t, to)
	if err := renamePOSIX(from, to); err != nil {
		t.Fatalf("POSIX rename over an open file: %v", err)
	}
	wantContent(t, to, "new")
	if _, err := os.Stat(from); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source left behind: %v", err)
	}
	if got, err := io.ReadAll(reader); err != nil || string(got) != "old" {
		t.Fatalf("the open handle reads %q (%v), want the old file", got, err)
	}
}

func TestRenameDoesNotWaitForAReaderSharingDelete(t *testing.T) {
	from, to := pair(t)
	openSharingDelete(t, to)
	if err := rename(from, to); err != nil {
		t.Fatal(err)
	}
	wantContent(t, to, "new")
}

func TestRenameWaitsForAReaderNotSharingDelete(t *testing.T) {
	from, to := pair(t)
	// Go's os.Open shares read and write, but not delete.
	reader, err := os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	if err := renamePOSIX(from, to); !isOpen(err) {
		reader.Close()
		t.Fatalf("POSIX rename over a file open without delete sharing: %v, want access denied or a sharing violation", err)
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

func TestRenamePOSIXTakesARelativePath(t *testing.T) {
	from, to := pair(t)
	t.Chdir(filepath.Dir(to))
	if err := renamePOSIX(filepath.Base(from), filepath.Base(to)); err != nil {
		t.Fatal(err)
	}
	wantContent(t, to, "new")
}

func TestRenameFallsBackWithoutPOSIXSemantics(t *testing.T) {
	for _, unsupported := range []error{windows.ERROR_INVALID_PARAMETER, windows.ERROR_NOT_SUPPORTED, windows.ERROR_INVALID_FUNCTION} {
		t.Run(unsupported.Error(), func(t *testing.T) {
			from, to := pair(t)
			calls := 0
			posix := func(string, string) error { calls++; return unsupported }
			reader, err := os.Open(to)
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				time.Sleep(30 * time.Millisecond)
				reader.Close()
			}()
			if err := replace(from, to, posix); err != nil {
				t.Fatal(err)
			}
			wantContent(t, to, "new")
			if calls < 2 {
				t.Fatalf("POSIX rename tried %d times, want a try on each retry", calls)
			}
		})
	}
}

func TestRenameGivesUpOnAFileThatStaysOpen(t *testing.T) {
	from, to := pair(t)
	reader, err := os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := rename(from, to); !isOpen(err) {
		t.Fatalf("err %v, want access denied or a sharing violation", err)
	}
	wantContent(t, to, "old")
	wantContent(t, from, "new")
}
