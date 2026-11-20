//go:build windows

package trash

import (
	"errors"
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shFileOperation = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// shFileOpStruct is SHFILEOPSTRUCTW, which 64-bit Windows lays out with its
// natural alignment, as Go does.
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete          = 0x3
	fofSilent         = 0x4
	fofNoConfirmation = 0x10
	fofAllowUndo      = 0x40
	fofNoErrorUI      = 0x400
)

// SystemRestore is how the system puts back what claudes moved to the trash,
// or "" when only Manifest can.
const SystemRestore = "Windows puts back what claudes moved to the Recycle Bin: open it, select the files, and choose Restore."

// Move moves path to the Recycle Bin. Windows deletes an item too big for the
// Recycle Bin outright, without asking, since claudes cannot show a prompt. It
// returns "", since Windows does not say where in the Recycle Bin the item
// went, so Manifest records nothing.
func Move(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// pFrom is a list of paths, each ending in a NUL, and the list in another.
	from, err := windows.UTF16FromString(abs)
	if err != nil {
		return "", err
	}
	from = append(from, 0)
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}
	code, _, _ := shFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if code != 0 {
		return "", fmt.Errorf("cannot move %s to the Recycle Bin: error %#x", path, code)
	}
	if op.fAnyOperationsAborted != 0 {
		return "", errors.New("moving " + path + " to the Recycle Bin was canceled")
	}
	return "", nil
}

// forget does nothing: Manifest holds no record of a file Windows moved.
func forget(string) {}
