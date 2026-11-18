//go:build windows

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// rename replaces to with from. POSIX semantics replace a file that other
// processes have open, if each opened it with delete sharing, as Node and Bun
// do. Without them, os.Rename cannot replace an open file at all. Either way a
// held file gets retries: Go's os.Open does not share delete, and antivirus
// scanners hold a file only for a moment.
func rename(from, to string) error { return replace(from, to, renamePOSIX) }

func replace(from, to string, posix func(from, to string) error) error {
	delay := 5 * time.Millisecond
	var err error
	for range renameAttempts {
		// Any error falls back, since builds and file systems that lack POSIX
		// semantics answer with different errors: invalid parameter, not
		// supported, or invalid function.
		if err = posix(from, to); err != nil {
			err = os.Rename(from, to)
		}
		if err == nil || !isOpen(err) {
			return err
		}
		time.Sleep(delay)
		delay = min(delay*2, 200*time.Millisecond)
	}
	return err
}

// isOpen reports whether a rename failed because another process has the
// target open.
func isOpen(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

// renameAttempts with the backoff above waits a little over a second in all.
const renameAttempts = 10

// fileRenameInfo is FILE_RENAME_INFO, whose name runs past the end of the
// struct.
type fileRenameInfo struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

// renamePOSIX renames with FILE_RENAME_FLAG_POSIX_SEMANTICS, which Windows 10
// 1607 and later support on NTFS. It unlinks the target at once, and processes
// that have it open keep reading the old file.
func renamePOSIX(from, to string) error {
	target, err := filepath.Abs(to)
	if err != nil {
		return err
	}
	name, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(source, windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)

	size := int(unsafe.Offsetof(fileRenameInfo{}.FileName)) + 2*len(name)
	// A uint64 buffer keeps the handle field aligned.
	buf := make([]uint64, (size+7)/8)
	info := (*fileRenameInfo)(unsafe.Pointer(&buf[0]))
	// x/sys names these after the kernel's FILE_RENAME_* flags, which have the
	// values of FILE_RENAME_FLAG_*.
	info.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.FileNameLength = uint32(2 * (len(name) - 1))
	copy(unsafe.Slice(&info.FileName[0], len(name)), name)
	return windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, (*byte)(unsafe.Pointer(info)), uint32(size))
}
