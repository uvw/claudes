//go:build darwin

package procs

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

// Executable is the path of the program a process runs. kern.procargs2 starts
// with the argument count and then the executable path, before argv, so a
// process that renames itself, as a Claude Code session does, still shows the
// file it runs.
func Executable(pid int) (string, error) {
	data, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", err
	}
	if len(data) < 5 {
		return "", errors.New("kern.procargs2 is too short")
	}
	path, _, _ := bytes.Cut(data[4:], []byte{0})
	return string(path), nil
}
