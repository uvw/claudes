//go:build linux

package procs

import (
	"os"
	"strconv"
	"strings"
)

// Executable is the path of the program a process runs. Once the file is
// removed, as when the native installer prunes an old version under a
// session that still runs it, the kernel adds " (deleted)" to the link.
func Executable(pid int) (string, error) {
	exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	return strings.TrimSuffix(exe, " (deleted)"), err
}
