//go:build linux

package procs

import (
	"os"
	"strconv"
)

// Executable is the path of the program a process runs.
func Executable(pid int) (string, error) {
	return os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
}
